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
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	capa "sigs.k8s.io/cluster-api-provider-aws/v2/api/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/irsa-operator/pkg/aws/scope"
	"github.com/giantswarm/irsa-operator/pkg/aws/services/acm"
	"github.com/giantswarm/irsa-operator/pkg/aws/services/cloudfront"
	"github.com/giantswarm/irsa-operator/pkg/aws/services/iam"
	"github.com/giantswarm/irsa-operator/pkg/aws/services/route53"
	"github.com/giantswarm/irsa-operator/pkg/aws/services/s3"
	irsaerrors "github.com/giantswarm/irsa-operator/pkg/errors"
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

func (s *Service) Reconcile(ctx context.Context, outRequeueAfter *time.Duration) error {
	var cfDomain string
	var cfOaiId string

	s.Scope.Logger().Info("Reconciling AWSCluster CR for IRSA")

	b := backoff.NewMaxRetries(3, 5*time.Second)
	err := s.S3.IsBucketReady(s.Scope.BucketName())
	// Check if S3 bucket exists
	if err != nil {
		createBucket := func() error {
			err := s.S3.CreateBucket(s.Scope.BucketName())
			if err != nil {
				s.Scope.Logger().Error(err, "Failed to create S3 bucket, retrying")
			}

			return err
		}
		err = backoff.Retry(createBucket, b)
		if err != nil {
			ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
			s.Scope.Logger().Error(err, "failed to create bucket")
			return err
		}
	}

	err = s.S3.EncryptBucket(s.Scope.BucketName())
	if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger().Error(err, "failed to encrypt bucket")
		return err
	}

	// Fetch custom tags from AWSCluster CR
	awsCluster := &capa.AWSCluster{}
	err = s.Client.Get(ctx, types.NamespacedName{Namespace: s.Scope.ClusterNamespace(), Name: s.Scope.ClusterName()}, awsCluster)
	if err != nil {
		return err
	}
	err = s.S3.CreateTags(s.Scope.BucketName(), awsCluster.Spec.AdditionalTags)
	if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger().Error(err, "failed to create tags")
		return err
	}

	aliases := make([]*string, 0)
	var cloudfrontCertificateARN string
	var hostedZoneID string

	distribution := &cloudfront.Distribution{}
	// Add Cloudfront only for non-China region
	if !key.IsChina(s.Scope.Region()) {
		cloudfrontAliasDomain := s.getCloudFrontAliasDomain()
		if cloudfrontAliasDomain != "" {
			// Ensure ACM certificate.
			certificateArn, err := s.ACM.EnsureCertificate(cloudfrontAliasDomain, awsCluster.Spec.AdditionalTags)
			if err != nil {
				ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
				s.Scope.Logger().Error(err, "failed to create ACM certificate")
				return err
			}

			// wait for certificate to be issued.
			issued, err := s.ACM.IsCertificateIssued(*certificateArn)
			if err != nil {
				ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
				s.Scope.Logger().Error(err, "failed to create ACM certificate")
				return err
			}

			hostedZoneID, err = s.Route53.FindPublicHostedZone(s.Scope.BaseDomain())
			if err != nil {
				ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
				s.Scope.Logger().Error(err, "failed to find route53 hosted zone ID")
				return err
			}

			if !issued {
				s.Scope.Logger().Info("ACM certificate is not issued yet")

				// Check if domain ownership is validated
				validated, err := s.ACM.IsValidated(*certificateArn)
				if err != nil {
					ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
					s.Scope.Logger().Error(err, "failed to check if ACM certificate's ownership is validated")
					return err
				}

				if !validated {
					// Check if DNS record is present
					cname, err := s.ACM.GetValidationCNAME(*certificateArn)
					if err != nil {
						ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
						s.Scope.Logger().Error(err, "failed to get ACM certificate's validation DNS record details")
						return err
					}

					err = s.Route53.EnsureDNSRecord(hostedZoneID, *cname)
					if err != nil {
						ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
						s.Scope.Logger().Error(err, "failed to create ACM certificate's validation DNS record")
						return err
					}

				}

				return microerror.Mask(certificateNotIssuedError)
			}

			aliases = append(aliases, &cloudfrontAliasDomain)
			cloudfrontCertificateARN = *certificateArn
		}

		distribution, err = s.Cloudfront.EnsureDistribution(cloudfront.DistributionConfig{CustomerTags: awsCluster.Spec.AdditionalTags, Aliases: aliases, CertificateArn: cloudfrontCertificateARN})
		if err != nil {
			ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
			s.Scope.Logger().Error(err, "failed to create cloudfront distribution")
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

	if len(aliases) > 0 && hostedZoneID != "" {
		for _, alias := range aliases {
			// Create IRSA Alias CNAME
			err = s.Route53.EnsureDNSRecord(hostedZoneID, route53.CNAME{Name: *alias, Value: key.EnsureTrailingDot(distribution.Domain)})
			if err != nil {
				ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
				s.Scope.Logger().Error(err, "failed to create cloudfront CNAME record")
				return err
			}
		}

		data["domainAlias"] = *aliases[0] //nolint:gosec
	}

	cfConfig := &v1.Secret{}
	err = s.Client.Get(ctx, types.NamespacedName{Namespace: s.Scope.ClusterNamespace(), Name: s.Scope.ConfigName()}, cfConfig)
	if apierrors.IsNotFound(err) {
		if err := irsaerrors.IsEmptyCloudfrontDistribution(distribution); err != nil {
			ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
			s.Scope.Logger().Error(err, "cloudfront distribution cannot be nil")
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
			s.Scope.Logger().Error(err, "failed to create OIDC cloudfront secret for cluster")
			return err
		}
		s.Scope.Logger().Info("Created OIDC cloudfront secret in k8s")

	} else if err != nil {
		return err
	}

	// Ensure CM is up to date
	if reflect.DeepEqual(cfConfig.Data, data) {
		s.Scope.Logger().Info("Secret is already up to date")
	} else {
		s.Scope.Logger().Info("Secret needs to be updated")

		cfConfig.StringData = data

		err = s.Client.Update(ctx, cfConfig)
		if err != nil {
			ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
			s.Scope.Logger().Error(err, "error updating secret")
			return err
		}

		s.Scope.Logger().Info("Secret updated successfully")
	}

	cfDomain = data["domain"]
	cfOaiId = data["originAccessIdentityId"]

	// Update S3 policy to allow access only via Cloudfront for non-China region
	if !key.IsChina(s.Scope.Region()) {
		uploadPolicy := func() error { return s.S3.UpdatePolicy(s.Scope.BucketName(), cfOaiId) }
		err = backoff.Retry(uploadPolicy, b)
		if err != nil {
			s.Scope.Logger().Error(err, "failed to upload policy")
			return err
		}
	}
	// Block public S3 access only for non-China region
	if !key.IsChina(s.Scope.Region()) {
		err = s.S3.BlockPublicAccess(s.Scope.BucketName())
		if err != nil {
			s.Scope.Logger().Error(err, "failed to block public access")
			return err
		}
	} else {
		err = s.S3.AllowPublicAccess(s.Scope.BucketName())
		if err != nil {
			s.Scope.Logger().Error(err, "failed to allow public access")
			return err
		}
	}

	privateKey, err := s.ServiceAccountSecret(ctx)
	if apierrors.IsNotFound(err) {
		s.Scope.Logger().Info("Service account is not ready yet, waiting ...")

		// Secret is handled by CAPI/kubeadm and may be available soon, so set a low requeue interval
		*outRequeueAfter = 30 * time.Second
		return nil
	}
	if err != nil {
		return err
	}

	uploadFiles := func() error {
		domain := cfDomain
		if len(aliases) > 0 {
			domain = *aliases[0] //nolint:gosec
		}
		return s.S3.UploadFiles(s.Scope.Release(), domain, s.Scope.BucketName(), privateKey)
	}
	err = backoff.Retry(uploadFiles, b)
	if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger().Error(err, "failed to upload files")
		return err
	}

	createOIDCProvider := func() error {
		var identityProviderURLs []string
		s3Endpoint := fmt.Sprintf("s3.%s.%s", s.Scope.Region(), key.AWSEndpoint(s.Scope.Region()))
		if key.IsChina(s.Scope.Region()) {
			identityProviderURLs = append(identityProviderURLs, util.EnsureHTTPS(fmt.Sprintf("%s/%s", s3Endpoint, s.Scope.BucketName())))
		}

		for _, alias := range aliases {
			identityProviderURLs = append(identityProviderURLs, util.EnsureHTTPS(*alias))
		}

		return s.IAM.EnsureOIDCProviders(identityProviderURLs, key.STSUrl(s.Scope.Region()), awsCluster.Spec.AdditionalTags)
	}
	err = backoff.Retry(createOIDCProvider, b)
	if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger().Error(err, "failed to create OIDC provider")
		return err
	}

	ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Set(0)
	s.Scope.Logger().Info("Finished reconciling on all resources.")
	return nil
}

func (s *Service) Delete(ctx context.Context) error {
	err := s.S3.DeleteFiles(s.Scope.BucketName())
	if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger().Error(err, "failed to delete S3 files")
		return err
	}
	err = s.S3.DeleteBucket(s.Scope.BucketName())
	if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger().Error(err, "failed to delete S3 bucket")
		return err
	}

	err = s.IAM.DeleteOIDCProviders()
	if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger().Error(err, "failed to delete OIDC provider")
		return err
	}

	if !key.IsChina(s.Scope.Region()) {
		cfConfig := &v1.Secret{}
		err = s.Client.Get(ctx, types.NamespacedName{Namespace: s.Scope.ClusterNamespace(), Name: s.Scope.ConfigName()}, cfConfig)
		if apierrors.IsNotFound(err) {
			s.Scope.Logger().Info("Secret for OIDC cloudfront does not exist anymore, skipping")
			return nil
		} else if err != nil {
			s.Scope.Logger().Error(err, "unexpected error")
			return err
		}

		data := cfConfig.Data
		cfDistributionId := string(data["distributionId"])
		cfOriginAccessIdentityId := string(data["originAccessIdentityId"])

		err = s.Cloudfront.DisableDistribution(cfDistributionId)
		if err != nil {
			s.Scope.Logger().Error(err, "failed to disable cloudfront distribution for cluster")
			return err
		}

		err = s.Cloudfront.DeleteDistribution(cfDistributionId)
		if errors.Is(err, &cloudfront.DistributionNotDisabledError{}) {
			return &CloudfrontDistributionNotDisabledError{}
		}
		if err != nil {
			return err
		}

		err = s.Cloudfront.DeleteOriginAccessIdentity(cfOriginAccessIdentityId)
		if err != nil {
			s.Scope.Logger().Error(err, "failed to delete cloudfront origin access identity for cluster")
			return err
		}

		err = s.Client.Delete(ctx, cfConfig, &client.DeleteOptions{Raw: &metav1.DeleteOptions{}})
		if apierrors.IsNotFound(err) {
			// OIDC cloudfront config map is already deleted
			// fall through
			s.Scope.Logger().Info("OIDC cloudfront config map for cluster not found, skipping deletion")
		} else if err != nil {
			ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
			s.Scope.Logger().Error(err, "failed to delete OIDC cloudfront config map for cluster")
			return microerror.Mask(err)
		}

		err = s.ACM.DeleteCertificate(s.getCloudFrontAliasDomain())
		if err != nil {
			s.Scope.Logger().Error(err, "error deleting ACM certificate")
			return err
		}
	}

	ctrlmetrics.Errors.DeleteLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace())
	s.Scope.Logger().Info("Finished deleting all resources.")

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

func (s *Service) getCloudFrontAliasDomain() string {
	return key.CloudFrontAlias(s.Scope.BaseDomain())
}
