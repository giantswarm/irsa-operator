package capa

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"reflect"
	"time"

	"github.com/giantswarm/backoff"
	"github.com/giantswarm/microerror"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"

	capi "sigs.k8s.io/cluster-api/api/v1beta1"

	"github.com/giantswarm/irsa-operator/pkg/aws/scope"
	"github.com/giantswarm/irsa-operator/pkg/aws/services/acm"
	"github.com/giantswarm/irsa-operator/pkg/aws/services/cloudfront"
	"github.com/giantswarm/irsa-operator/pkg/aws/services/iam"
	"github.com/giantswarm/irsa-operator/pkg/aws/services/route53"
	"github.com/giantswarm/irsa-operator/pkg/aws/services/s3"
	"github.com/giantswarm/irsa-operator/pkg/errors"
	"github.com/giantswarm/irsa-operator/pkg/key"
	ctrlmetrics "github.com/giantswarm/irsa-operator/pkg/metrics"
	"github.com/giantswarm/irsa-operator/pkg/util"
)

type Service struct {
	Client client.Client
	Scope  *scope.ClusterScope

	ACM        *acm.Service
	Cloudfront *cloudfront.Service
	IAM        *iam.Service
	Route53    *route53.Service
	S3         *s3.Service
}

func New(scope *scope.ClusterScope, client client.Client) *Service {
	return &Service{
		Scope:  scope,
		Client: client,

		ACM:        acm.NewService(scope),
		Cloudfront: cloudfront.NewService(scope),
		IAM:        iam.NewService(scope),
		Route53:    route53.NewService(scope),
		S3:         s3.NewService(scope),
	}
}
func (s *Service) Reconcile(ctx context.Context) error {
	var cfDomain string
	var cfOaiId string

	s.Scope.Info("Reconciling AWSCluster CR for IRSA")
	privateKey, err := s.ServiceAccountSecret(ctx)
	if apierrors.IsNotFound(err) {
		s.Scope.Info("Service account is not ready yet, waiting ...")
		return nil
	} else if err != nil {
		return err
	}

	b := backoff.NewMaxRetries(3, 5*time.Second)

	err = s.S3.IsBucketReady(s.Scope.BucketName())
	// Check if S3 bucket exists
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
	var cloudfrontAliasDomain string
	cloudfrontAliasDomain = key.CloudFrontAlias(s.Scope.ClusterName(), s.Scope.Installation(), s.Scope.Region())

	err = s.S3.CreateTags(s.Scope.BucketName(), customerTags)
	if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger.Error(err, "failed to create tags")
		return err
	}

	distribution := &cloudfront.Distribution{}
	// Add Cloudfront only for non-China region
	if !key.IsChina(s.Scope.Region()) {

		aliases := make([]*string, 0)

		if cloudfrontAliasDomain != "" {
			// Ensure ACM certificate.
			certificateArn, err := s.ACM.EnsureCertificate(cloudfrontAliasDomain, customerTags)
			if err != nil {
				ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
				s.Scope.Logger.Error(err, "failed to create ACM certificate")
				return err
			}

			// wait for certificate to be issued.
			issued, err := s.ACM.IsCertificateIssued(*certificateArn)
			if err != nil {
				ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
				s.Scope.Logger.Error(err, "failed to create ACM certificate")
				return err
			}

			hostedZoneID, err := s.Route53.FindHostedZone(key.BaseDomain(s.Scope.ClusterName(), s.Scope.Installation(), s.Scope.Region()))
			if err != nil {
				ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
				s.Scope.Logger.Error(err, "failed to find route53 hosted zone ID")
				return err
			}

			if !issued {
				s.Scope.Logger.Info("ACM certificate is not issued yet")

				// Check if domain ownership is validated
				validated, err := s.ACM.IsValidated(*certificateArn)
				if err != nil {
					ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
					s.Scope.Logger.Error(err, "failed to check if ACM certificate's ownership is validated")
					return err
				}

				if !validated {
					// Check if DNS record is present
					cname, err := s.ACM.GetValidationCNAME(*certificateArn)
					if err != nil {
						ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
						s.Scope.Logger.Error(err, "failed to get ACM certificate's validation DNS record details")
						return err
					}

					err = s.Route53.EnsureDNSRecord(hostedZoneID, *cname)
					if err != nil {
						ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
						s.Scope.Logger.Error(err, "failed to create ACM certificate's validation DNS record")
						return err
					}

				}

				return microerror.Mask(certificateNotIssuedError)
			}

			aliases = append(aliases, &cloudfrontAliasDomain)
		}

		distribution, err = s.Cloudfront.EnsureDistribution(cloudfront.DistributionConfig{CustomerTags: customerTags, Aliases: aliases})
		if err != nil {
			ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
			s.Scope.Logger.Error(err, "failed to create cloudfront distribution")
			return err
		}
	}

	// kubeadmconfig only support secrets for now, therefore we need to store Cloudfront config as a secret, see
	// https://github.com/giantswarm/cluster-api-app/blob/master/helm/cluster-api/files/bootstrap/patches/versions/v1beta1/kubeadmconfigs.bootstrap.cluster.x-k8s.io.yaml#L307-L325

	data := map[string]string{
		"arn":                    distribution.ARN,
		"domain":                 distribution.Domain,
		"distributionId":         distribution.DistributionId,
		"originAccessIdentityId": distribution.OriginAccessIdentityId,
	}

	cfConfig := &v1.Secret{}
	err = s.Client.Get(ctx, types.NamespacedName{Namespace: s.Scope.ClusterNamespace(), Name: s.Scope.ConfigName()}, cfConfig)
	if apierrors.IsNotFound(err) {
		if err := errors.IsEmptyCloudfrontDistribution(distribution); err != nil {
			ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
			s.Scope.Logger.Error(err, "cloudfront distribution cannot be nil")
			return err
		}

		// create new OIDC Cloudfront config
		cfConfig := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      s.Scope.ConfigName(),
				Namespace: s.Scope.ClusterNamespace(),
			},
			StringData: data,
		}

		if err := s.Client.Create(ctx, cfConfig); err != nil {
			ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
			s.Scope.Logger.Error(err, "failed to create OIDC cloudfront secret for cluster")
			return err
		}
		s.Scope.Logger.Info("Created OIDC cloudfront secret in k8s")

	} else if err != nil {
		return err
	}

	// Ensure CM is up to date
	if reflect.DeepEqual(cfConfig.Data, data) {
		s.Scope.Logger.Info("Secret is already up to date")
	} else {
		s.Scope.Logger.Info("Secret needs to be updated")

		cfConfig.StringData = data

		err = s.Client.Update(ctx, cfConfig)
		if err != nil {
			ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
			s.Scope.Logger.Error(err, "error updating secret")
			return err
		}

		s.Scope.Logger.Info("Secret updated successfully")
	}

	cfDomain = data["domain"]
	cfOaiId = data["originAccessIdentityId"]

	uploadFiles := func() error {
		return s.S3.UploadFiles(s.Scope.Release(), cfDomain, s.Scope.BucketName(), privateKey)
	}
	err = backoff.Retry(uploadFiles, b)
	if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger.Error(err, "failed to upload files")
		return err
	}

	// Update S3 policy to allow access only via Cloudfront for non-China region
	if !key.IsChina(s.Scope.Region()) {
		uploadPolicy := func() error { return s.S3.UpdatePolicy(s.Scope.BucketName(), cfOaiId) }
		err = backoff.Retry(uploadPolicy, b)
		if err != nil {
			s.Scope.Logger.Error(err, "failed to upload policy")
			return err
		}
	}
	// Block public S3 access only for non-China region
	if !key.IsChina(s.Scope.Region()) {
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

		return s.IAM.EnsureOIDCProvider(identityProviderURL, key.STSUrl(s.Scope.Region()))
	}
	err = backoff.Retry(createOIDCProvider, b)
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

func (s *Service) Delete(ctx context.Context) error {
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
	cfConfig := &v1.Secret{}

	if !key.IsChina(s.Scope.Region()) {
		err = s.Client.Get(ctx, types.NamespacedName{Namespace: s.Scope.ClusterNamespace(), Name: s.Scope.ConfigName()}, cfConfig)
		if apierrors.IsNotFound(err) {
			s.Scope.Logger.Info("Secret for OIDC cloudfront does not exist anymore, skipping")
			return nil
		} else if err != nil {
			s.Scope.Logger.Error(err, "unexpected error")
			return err
		}

		data := cfConfig.Data
		cfDomain = string(data["domain"])
		cfDistributionId = string(data["distributionId"])
		cfOriginAccessIdentityId = string(data["originAccessIdentityId"])
	}

	err = s.IAM.DeleteOIDCProvider(s.Scope.Release(), cfDomain, s.Scope.AccountID(), s.Scope.BucketName(), s.Scope.Region())
	if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger.Error(err, "failed to delete OIDC provider")
		return err
	}

	if !key.IsChina(s.Scope.Region()) {
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

func (s *Service) ServiceAccountSecret(ctx context.Context) (*rsa.PrivateKey, error) {
	oidcSecret := &v1.Secret{}
	err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Scope.ClusterNamespace(), Name: s.Scope.ClusterName() + "-sa"}, oidcSecret)
	if err != nil {
		return nil, err
	}
	privBytes := oidcSecret.Data["tls.key"]
	block, _ := pem.Decode(privBytes)
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return privateKey, nil
}
