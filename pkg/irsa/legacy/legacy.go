package legacy

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

	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/irsa-operator/pkg/aws/scope"
	"github.com/giantswarm/irsa-operator/pkg/aws/services/acm"
	"github.com/giantswarm/irsa-operator/pkg/aws/services/cloudfront"
	"github.com/giantswarm/irsa-operator/pkg/aws/services/iam"
	"github.com/giantswarm/irsa-operator/pkg/aws/services/route53"
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
	if err != nil {
		return err
	}

	b := backoff.NewMaxRetries(3, 5*time.Second)

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

	baseDomain, err := key.BaseDomain(*cluster)
	if err != nil {
		return err
	}

	customerTags := key.GetCustomerTags(cluster)
	aliases := make([]*string, 0)
	cloudfrontCertificateARN := ""

	err = s.S3.CreateTags(s.Scope.BucketName(), customerTags)
	if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger.Error(err, "failed to create tags")
		return err
	}

	// Cloudfront only for non-China region and v18.x.x release or higher
	if !key.IsChina(s.Scope.Region()) && key.IsV18Release(s.Scope.Release()) || (s.Scope.MigrationNeeded() && !key.IsChina(s.Scope.Region())) {
		var hostedZoneID string
		cloudfrontAliasDomain := key.CloudFrontAlias(baseDomain)
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
				s.Scope.Logger.Error(err, "failed to check if ACM certificate is issued")
				return err
			}

			hostedZoneID, err = s.Route53.FindHostedZone(baseDomain)
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
			cloudfrontCertificateARN = *certificateArn
		}

		distribution, err := s.Cloudfront.EnsureDistribution(cloudfront.DistributionConfig{
			Aliases:        aliases,
			CertificateArn: cloudfrontCertificateARN,
			CustomerTags:   customerTags,
		})
		if err != nil {
			ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
			s.Scope.Logger.Error(err, "failed to create cloudfront distribution")
			return err
		}

		data := map[string]string{
			"arn":                    distribution.ARN,
			"domain":                 distribution.Domain,
			"distributionId":         distribution.DistributionId,
			"originAccessIdentityId": distribution.OriginAccessIdentityId,
		}

		if len(aliases) > 0 && hostedZoneID != "" {
			for _, alias := range aliases {
				// Create IRSA Alias CNAME
				err = s.Route53.EnsureDNSRecord(hostedZoneID, route53.CNAME{Name: *alias, Value: key.EnsureTrailingDot(distribution.Domain)})
				if err != nil {
					ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
					s.Scope.Logger.Error(err, "failed to create cloudfront CNAME record")
					return err
				}
			}

			data["domainAlias"] = *aliases[0]
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
			cfConfig = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      s.Scope.ConfigName(),
					Namespace: s.Scope.ClusterNamespace(),
				},
				Data: data,
			}

			if err := s.Client.Create(ctx, cfConfig); err != nil {
				ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
				s.Scope.Logger.Error(err, "failed to create OIDC cloudfront config map for cluster")
				return err
			}
			s.Scope.Logger.Info("Created OIDC cloudfront config map in k8s")
		} else if err != nil {
			return err
		}

		// Ensure CM is up-to-date.
		if reflect.DeepEqual(cfConfig.Data, data) {
			s.Scope.Logger.Info("Configmap is already up to date")
		} else {
			s.Scope.Logger.Info("Configmap needs to be updated")

			cfConfig.Data = data

			err = s.Client.Update(ctx, cfConfig)
			if err != nil {
				ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
				s.Scope.Logger.Error(err, "error updating configmap")
				return err
			}

			s.Scope.Logger.Info("Configmap updated successfully")
		}

		cfDomain = distribution.Domain
		cfOaiId = data["originAccessIdentityId"]
	}

	uploadFiles := func() error {
		domain := cfDomain
		if len(aliases) > 0 {
			domain = *aliases[0]
		}
		return s.S3.UploadFiles(s.Scope.Release(), domain, s.Scope.BucketName(), privateKey)
	}
	err = backoff.Retry(uploadFiles, b)
	if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
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
		var identityProviderURLs []string
		s3Endpoint := fmt.Sprintf("s3.%s.%s", s.Scope.Region(), key.AWSEndpoint(s.Scope.Region()))
		if (key.IsV18Release(s.Scope.Release()) && !key.IsChina(s.Scope.Region())) || (s.Scope.MigrationNeeded() && !key.IsChina(s.Scope.Region())) {
			identityProviderURLs = append(identityProviderURLs, util.EnsureHTTPS(cfDomain))
		} else {
			identityProviderURLs = append(identityProviderURLs, util.EnsureHTTPS(fmt.Sprintf("%s/%s", s3Endpoint, s.Scope.BucketName())))
		}

		for _, alias := range aliases {
			identityProviderURLs = append(identityProviderURLs, util.EnsureHTTPS(*alias))
		}

		return s.IAM.EnsureOIDCProviders(identityProviderURLs, key.STSUrl(s.Scope.Region()), customerTags)
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

		cfDistributionId = cfConfig.Data["distributionId"]
		cfOriginAccessIdentityId = cfConfig.Data["originAccessIdentityId"]
	}

	err = s.IAM.DeleteOIDCProviders()
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

		// Fetch custom tags from Cluster CR
		cluster := &capi.Cluster{}
		err = s.Client.Get(ctx, types.NamespacedName{Namespace: s.Scope.ClusterNamespace(), Name: s.Scope.ClusterName()}, cluster)
		if apierrors.IsNotFound(err) {
			// fallthrough
		} else if err != nil {
			return err
		}

		baseDomain, err := key.BaseDomain(*cluster)
		if err != nil {
			return err
		}

		cloudFrontAliasDomain := key.CloudFrontAlias(baseDomain)
		if cloudFrontAliasDomain != "" {
			err = s.ACM.DeleteCertificate(cloudFrontAliasDomain)
			if err != nil {
				s.Scope.Logger.Error(err, "error deleting ACM certificate")
				return err
			}
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
