package legacy

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/giantswarm/backoff"
	"github.com/giantswarm/microerror"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/irsa-operator/pkg/aws/scope"
	"github.com/giantswarm/irsa-operator/pkg/aws/services/cloudfront"
	"github.com/giantswarm/irsa-operator/pkg/aws/services/iam"
	"github.com/giantswarm/irsa-operator/pkg/aws/services/s3"
	"github.com/giantswarm/irsa-operator/pkg/errors"
	"github.com/giantswarm/irsa-operator/pkg/key"
	ctrlmetrics "github.com/giantswarm/irsa-operator/pkg/metrics"
	"github.com/giantswarm/irsa-operator/pkg/pkcs"
	"github.com/giantswarm/irsa-operator/pkg/util"
)

type Service struct {
	Client client.Client
	Scope  *scope.ClusterScope

	Cloudfront *cloudfront.Service
	IAM        *iam.Service
	S3         *s3.Service
}

func New(scope *scope.ClusterScope, client client.Client) *Service {
	return &Service{
		Scope:  scope,
		Client: client,

		Cloudfront: cloudfront.NewService(scope),
		IAM:        iam.NewService(scope),
		S3:         s3.NewService(scope),
	}
}
func (s *Service) Reconcile(ctx context.Context) error {
	var cfDomain string
	var cfOaiId string

	s.Scope.Info("Reconciling AWSCluster CR for IRSA")
	privateKey, err := s.ServiceAccountSecret(ctx)
	if err != nil {
		return err
	}

	b := backoff.NewMaxRetries(20, 15*time.Second)

	err = s.S3.IsBucketReady(s.Scope.BucketName())
	// check if bucket exists
	if err != nil {
		createBucket := func() error {
			err := s.S3.CreateBucket(s.Scope.BucketName())
			if err != nil {
				s.Scope.Logger.Error(err, "Failed to create S3 bucket, retrying ")
			}

			return err
		}
		err = backoff.Retry(createBucket, b)
		if err != nil {
			ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
			s.Scope.Logger.Error(err, "failed to create bucket")
			return err
		}
	}

	err = s.S3.EncryptBucket(s.Scope.BucketName())
	if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger.Error(err, "failed to encrypt bucket")
		return err
	}

	// Fetch custom tags from Cluster CR
	cluster := &capi.Cluster{}
	err = s.Client.Get(ctx, types.NamespacedName{Namespace: s.Scope.ClusterNamespace(), Name: s.Scope.ClusterName()}, cluster)
	if apierrors.IsNotFound(err) {
		// fallthrough
	} else if err != nil {
		return err
	}

	customerTags := key.GetCustomerTags(cluster)

	err = s.S3.CreateTags(s.Scope.BucketName(), customerTags)
	if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger.Error(err, "failed to create tags")
		return err
	}

	// Cloudfront only for non-China region and v18.x.x release or higher
	if !key.IsChina(s.Scope.Region()) && key.IsV18Release(s.Scope.Release()) || (s.Scope.MigrationNeeded() && !key.IsChina(s.Scope.Region())) {
		distribution, err := s.Cloudfront.CreateDistribution(s.Scope.AccountID(), customerTags)
		if err != nil {
			ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
			s.Scope.Logger.Error(err, "failed to create cloudfront distribution")
			return err
		}

		cfConfig := &v1.ConfigMap{}
		err = s.Client.Get(ctx, types.NamespacedName{Namespace: s.Scope.ClusterNamespace(), Name: s.Scope.ConfigName()}, cfConfig)
		if apierrors.IsNotFound(err) {
			if err := errors.IsEmptyCloudfrontDistribution(distribution); err != nil {
				ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
				s.Scope.Logger.Error(err, "cloudfront distribution cannot be nil")
				return err
			}

			// create new OIDC Cloudfront config
			cfConfig := &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      s.Scope.ConfigName(),
					Namespace: s.Scope.ClusterNamespace(),
				},
				Data: map[string]string{
					"arn":                    distribution.ARN,
					"domain":                 distribution.Domain,
					"distributionId":         distribution.DistributionId,
					"originAccessIdentityId": distribution.OriginAccessIdentityId,
				},
			}

			if err := s.Client.Create(ctx, cfConfig); err != nil {
				ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
				s.Scope.Logger.Error(err, "failed to create OIDC cloudfront config map for cluster")
				return err
			}
			s.Scope.Logger.Info("Created OIDC cloudfront config map in k8s")
			cfDomain = distribution.Domain
			cfOaiId = distribution.OriginAccessIdentityId

		} else if err == nil {
			cfDomain = cfConfig.Data["domain"]
			if cfDomain == "" {
				s.Scope.Logger.Error(err, "failed to get OIDC cloudfront domain for cluster")
				return err
			}
			cfOaiId = cfConfig.Data["originAccessIdentityId"]
			if cfDomain == "" {
				s.Scope.Logger.Error(err, "failed to get OIDC cloudfront OAI id for cluster")
				return err
			}
		} else {
			return err
		}
	}

	uploadFiles := func() error {
		return s.S3.UploadFiles(s.Scope.Release(), cfDomain, s.Scope.BucketName(), privateKey)
	}
	err = backoff.Retry(uploadFiles, b)
	if err != nil {
		s.Scope.Logger.Error(err, "failed to upload files")
		return err
	}

	// restrict access only for non-China region and v18.x.x release or higher
	if (!key.IsChina(s.Scope.Region()) && key.IsV18Release(s.Scope.Release())) || (s.Scope.MigrationNeeded() && !key.IsChina(s.Scope.Region())) {
		uploadPolicy := func() error { return s.S3.UpdatePolicy(s.Scope.BucketName(), cfOaiId) }
		err = backoff.Retry(uploadPolicy, b)
		if err != nil {
			s.Scope.Logger.Error(err, "failed to upload policy")
			return err
		}
	}
	// restrict access only for non-China region and v18.x.x release or higher
	if (!key.IsChina(s.Scope.Region()) && key.IsV18Release(s.Scope.Release())) || (s.Scope.MigrationNeeded() && !key.IsChina(s.Scope.Region())) {
		err = s.S3.BlockPublicAccess(s.Scope.BucketName())
		if err != nil {
			s.Scope.Logger.Error(err, "failed to block public access")
			return err
		}
	}

	createOIDCProvider := func() error {
		var identityProviderURL string
		s3Endpoint := fmt.Sprintf("s3.%s.%s", s.Scope.Region(), key.AWSEndpoint(s.Scope.Region()))
		if (key.IsV18Release(s.Scope.Release()) && !key.IsChina(s.Scope.Region())) || (s.Scope.MigrationNeeded() && !key.IsChina(s.Scope.Region())) {
			identityProviderURL = fmt.Sprintf("https://%s", cfDomain)
		} else {
			identityProviderURL = fmt.Sprintf("https://%s/%s", s3Endpoint, s.Scope.BucketName())
		}

		return s.IAM.EnsureOIDCProvider(identityProviderURL, key.AWSEndpoint(s.Scope.Region()))
	}
	n := func(err error, d time.Duration) {
		s.Scope.Logger.Info("level", "warning", "message", fmt.Sprintf("retrying backoff in '%s' due to error", d.String()), "stack", fmt.Sprintf("%#v", err))
	}
	err = backoff.RetryNotify(createOIDCProvider, b, n)
	if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger.Error(err, "failed to create OIDC provider")
		return err
	}

	err = s.IAM.CreateOIDCTags(s.Scope.Release(), cfDomain, s.Scope.AccountID(), s.Scope.BucketName(), s.Scope.Region(), customerTags)
	if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger.Error(err, "failed to create tags")
		return err
	}

	oidcTags, err := s.IAM.ListCustomerOIDCTags(s.Scope.Release(), cfDomain, s.Scope.AccountID(), s.Scope.BucketName(), s.Scope.Region())
	if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger.Error(err, "failed to list OIDC provider tags")
		return err
	}

	if diff := util.MapsDiff(customerTags, oidcTags); diff != nil {
		s.Scope.Logger.Info("Cluster tags differ from current OIDC tags")
		if err := s.IAM.RemoveOIDCTags(s.Scope.Release(), cfDomain, s.Scope.AccountID(), s.Scope.BucketName(), s.Scope.Region(), diff); err != nil {
			ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
			s.Scope.Logger.Error(err, "failed to remove tags")
			return microerror.Mask(err)
		}
	}

	ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Set(0)
	s.Scope.Logger.Info("Finished reconciling on all resources.")
	return nil
}

func (s Service) Delete(ctx context.Context) error {
	err := s.S3.DeleteFiles(s.Scope.BucketName())
	if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger.Error(err, "failed to delete S3 files")
		return err
	}
	err = s.S3.DeleteBucket(s.Scope.BucketName())
	if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger.Error(err, "failed to delete S3 bucket")
		return err
	}

	var cfDomain string
	var cfDistributionId string
	var cfOriginAccessIdentityId string
	cfConfig := &v1.ConfigMap{}

	if (!key.IsChina(s.Scope.Region()) && key.IsV18Release(s.Scope.Release())) || (s.Scope.MigrationNeeded() && !key.IsChina(s.Scope.Region())) {
		err = s.Client.Get(ctx, types.NamespacedName{Namespace: s.Scope.ClusterNamespace(), Name: s.Scope.ConfigName()}, cfConfig)
		if apierrors.IsNotFound(err) {
			s.Scope.Logger.Info("Configmap for OIDC cloudfront does not exist anymore, skipping")
			return nil
		} else if err != nil {
			s.Scope.Logger.Error(err, "unexpected error")
			return err
		}

		cfDomain = cfConfig.Data["domain"]
		cfDistributionId = cfConfig.Data["distributionId"]
		cfOriginAccessIdentityId = cfConfig.Data["originAccessIdentityId"]
	}

	err = s.IAM.DeleteOIDCProvider(s.Scope.Release(), cfDomain, s.Scope.AccountID(), s.Scope.BucketName(), s.Scope.Region())
	if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger.Error(err, "failed to delete OIDC provider")
		return err
	}

	oidcSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.Scope.SecretName(),
			Namespace: s.Scope.ClusterNamespace(),
		},
	}
	err = s.Client.Delete(ctx, oidcSecret, &client.DeleteOptions{Raw: &metav1.DeleteOptions{}})
	if apierrors.IsNotFound(err) {
		// OIDC secret is already deleted
		// fall through
		s.Scope.Logger.Info("OIDC service account secret for cluster not found, skipping deletion")
	} else if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger.Error(err, "failed to delete OIDC service account secret for cluster")
		return err
	}

	if (!key.IsChina(s.Scope.Region()) && key.IsV18Release(s.Scope.Release())) || (s.Scope.MigrationNeeded() && !key.IsChina(s.Scope.Region())) {
		err = s.Cloudfront.DisableDistribution(cfDistributionId)
		if err != nil {
			s.Scope.Logger.Error(err, "failed to disable cloudfront distribution for cluster")
			return err
		}

		deleteDistribution := func() error {
			err = s.Cloudfront.DeleteDistribution(cfDistributionId)
			if err != nil {
				return err
			}
			return nil
		}

		err = backoff.Retry(deleteDistribution, backoff.NewMaxRetries(30, 1*time.Minute))
		if err != nil {
			s.Scope.Logger.Error(err, "failed to delete cloudfront distribution")
			return err
		}

		err = s.Cloudfront.DeleteOriginAccessIdentity(cfOriginAccessIdentityId)
		if err != nil {
			s.Scope.Logger.Error(err, "failed to delete cloudfront origin access identity for cluster")
			return err
		}

		err = s.Client.Delete(ctx, cfConfig, &client.DeleteOptions{Raw: &metav1.DeleteOptions{}})
		if apierrors.IsNotFound(err) {
			// OIDC cloudfront config map is already deleted
			// fall through
			s.Scope.Logger.Info("OIDC cloudfront config map for cluster not found, skipping deletion")
		} else if err != nil {
			ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
			s.Scope.Logger.Error(err, "failed to delete OIDC cloudfront config map for cluster")
			return microerror.Mask(err)
		}
	}

	ctrlmetrics.Errors.DeleteLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace())
	s.Scope.Logger.Info("Finished deleting all resources.")

	return nil
}

func (s Service) ServiceAccountSecret(ctx context.Context) (*rsa.PrivateKey, error) {
	oidcSecret := &v1.Secret{}
	err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Scope.ClusterNamespace(), Name: s.Scope.SecretName()}, oidcSecret)
	if apierrors.IsNotFound(err) {
		// create new OIDC service account secret
		privateSignerKey, publicSignerKey, pkey, err := pkcs.GenerateKeys()
		if err != nil {
			return nil, err
		}

		oidcSecret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      s.Scope.SecretName(),
				Namespace: s.Scope.ClusterNamespace(),
			},
			StringData: map[string]string{
				"key": privateSignerKey,
				"pub": publicSignerKey,
			},
			Type: v1.SecretTypeOpaque,
		}

		if err := s.Client.Create(ctx, oidcSecret); err != nil {
			ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
			s.Scope.Logger.Error(err, "failed to create OIDC service account secret for cluster")
			return nil, err
		}
		s.Scope.Logger.Info("Created secret signer keys in k8s")

		return pkey, nil
	} else if err == nil {
		// if secret already exists, parse the private key
		privBytes := oidcSecret.Data["key"]
		block, _ := pem.Decode(privBytes)
		privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		return privateKey, nil
	} else {
		return nil, err
	}
}
