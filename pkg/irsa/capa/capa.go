package capa

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
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
	"github.com/giantswarm/irsa-operator/pkg/aws/services/cloudfront"
	"github.com/giantswarm/irsa-operator/pkg/aws/services/iam"
	"github.com/giantswarm/irsa-operator/pkg/aws/services/s3"
	"github.com/giantswarm/irsa-operator/pkg/key"
	ctrlmetrics "github.com/giantswarm/irsa-operator/pkg/metrics"
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

	err = s.S3.CreateTags(s.Scope.BucketName(), customerTags)
	if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger.Error(err, "failed to create tags")
		return err
	}

	distribution := &cloudfront.Distribution{}
	// Add Cloudfront only for non-China region
	if !key.IsChina(s.Scope.Region()) {
		distribution, err = s.Cloudfront.CreateDistribution(s.Scope.AccountID(), customerTags)
		if err != nil {
			ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
			s.Scope.Logger.Error(err, "failed to create cloudfront distribution")
			return err
		}
	}

	// kubeadmconfig only support secrets for now, therefore we need to store Cloudfront config as a secret, see
	// https://github.com/giantswarm/cluster-api-app/blob/master/helm/cluster-api/files/bootstrap/patches/versions/v1beta1/kubeadmconfigs.bootstrap.cluster.x-k8s.io.yaml#L307-L325
	cfConfig := &v1.Secret{}
	err = s.Client.Get(ctx, types.NamespacedName{Namespace: s.Scope.ClusterNamespace(), Name: s.Scope.ConfigName()}, cfConfig)
	if apierrors.IsNotFound(err) {
		// create new OIDC Cloudfront config
		cfConfig := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      s.Scope.ConfigName(),
				Namespace: s.Scope.ClusterNamespace(),
			},
			StringData: map[string]string{
				"arn":                    distribution.ARN,
				"domain":                 distribution.Domain,
				"distributionId":         distribution.DistributionId,
				"originAccessIdentityId": distribution.OriginAccessIdentityId,
			},
		}

		if err := s.Client.Create(ctx, cfConfig); err != nil {
			ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
			s.Scope.Logger.Error(err, "failed to create OIDC cloudfront secret for cluster")
			return err
		}
		s.Scope.Logger.Info("Created OIDC cloudfront secret in k8s")
		cfDomain = distribution.Domain
		cfOaiId = distribution.OriginAccessIdentityId

	} else if err == nil {
		cfDomain = cfConfig.StringData["domain"]
		if cfDomain == "" {
			s.Scope.Logger.Error(err, "failed to get OIDC cloudfront domain for cluster")
			return err
		}
		cfOaiId = cfConfig.StringData["originAccessIdentityId"]
		if cfDomain == "" {
			s.Scope.Logger.Error(err, "failed to get OIDC cloudfront OAI id for cluster")
			return err
		}
	} else {
		return err
	}

	uploadFiles := func() error {
		return s.S3.UploadFiles(s.Scope.Release(), cfDomain, s.Scope.BucketName(), privateKey)
	}
	err = backoff.Retry(uploadFiles, b)
	if err != nil {
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
		return s.IAM.CreateOIDCProvider(s.Scope.Release(), cfDomain, s.Scope.BucketName(), s.Scope.Region())
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

		cfDomain = cfConfig.StringData["domain"]
		cfDistributionId = cfConfig.StringData["distributionId"]
		cfOriginAccessIdentityId = cfConfig.StringData["originAccessIdentityId"]
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
		s.Scope.Logger.Error(err, "failed to get service account secret")
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
