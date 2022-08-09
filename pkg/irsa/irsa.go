package irsa

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"time"

	"github.com/blang/semver"
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
	"github.com/giantswarm/irsa-operator/pkg/pkcs"
	"github.com/giantswarm/irsa-operator/pkg/util"
)

type IRSAService struct {
	Client client.Client
	Scope  *scope.ClusterScope

	Cloudfront *cloudfront.Service
	IAM        *iam.Service
	S3         *s3.Service
}

func New(scope *scope.ClusterScope, client client.Client) *IRSAService {
	return &IRSAService{
		Scope:  scope,
		Client: client,

		Cloudfront: cloudfront.NewService(scope),
		IAM:        iam.NewService(scope),
		S3:         s3.NewService(scope),
	}
}

func (s *IRSAService) Reconcile(ctx context.Context) error {
	var privateKey *rsa.PrivateKey
	var cfDomain string
	var cfOaiId string

	s.Scope.Info("Reconciling AWSCluster CR for IRSA")
	release, _ := semver.New(s.Scope.ReleaseVersion())

	oidcSecret := &v1.Secret{}
	err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Scope.ClusterNamespace(), Name: s.Scope.SecretName()}, oidcSecret)
	if apierrors.IsNotFound(err) {
		// create new OIDC service account secret
		privateSignerKey, publicSignerKey, pkey, err := pkcs.GenerateKeys()
		if err != nil {
			return err
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
			return err
		}
		s.Scope.Logger.Info("Created secret signer keys in k8s")

		privateKey = pkey
	} else if err == nil {
		// if secret already exists, parse the private key
		privBytes := oidcSecret.Data["key"]
		block, _ := pem.Decode(privBytes)
		privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return err
		}
	} else {
		return err
	}

	b := backoff.NewMaxRetries(10, 15*time.Second)

	err = s.S3.IsBucketReady(s.Scope.BucketName())
	// check if bucket exists
	if err != nil {
		s.Scope.Logger.Info("Creating S3 bucket", s.Scope.BucketName())
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
		s.Scope.Logger.Info("Created S3 bucket", s.Scope.BucketName())

	} else {
		s.Scope.Logger.Info("S3 bucket already exists, skipping creation", s.Scope.BucketName())
	}

	err = s.S3.EncryptBucket(s.Scope.BucketName())
	if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger.Error(err, "failed to encrypt bucket")
		return err
	}
	s.Scope.Logger.Info("Encrypted S3 bucket", s.Scope.BucketName())

	// Fetch custom tags from Cluster CR
	cluster := &capi.Cluster{}
	err = s.Client.Get(ctx, types.NamespacedName{Namespace: s.Scope.ClusterNamespace(), Name: s.Scope.ClusterName()}, cluster)
	if apierrors.IsNotFound(err) {
		// fallthrough
	} else if err != nil {
		return err
	}

	customerTags := getCustomerTags(cluster)

	err = s.S3.CreateTags(s.Scope.BucketName(), customerTags)
	if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger.Error(err, "failed to create tags")
		return err
	}
	s.Scope.Logger.Info("Created tags for S3 bucket", s.Scope.BucketName())

	// only restrict access when IRSA is used via Cloudfront in v18
	if key.IsV18Release(release) {
		err = s.S3.BlockPublicAccess(s.Scope.BucketName())
		if err != nil {
			s.Scope.Logger.Error(err, "failed to block public access")
			return err
		}
		s.Scope.Logger.Info("Blocked public access for S3 bucket", s.Scope.BucketName())
	}

	distribution, err := s.Cloudfront.CreateDistribution(s.Scope.AccountID())
	if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger.Error(err, "failed to create cloudfront distribution")
		return err
	}

	cfConfig := &v1.ConfigMap{}
	err = s.Client.Get(ctx, types.NamespacedName{Namespace: s.Scope.ClusterNamespace(), Name: s.Scope.ConfigName()}, cfConfig)
	if apierrors.IsNotFound(err) {
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

	uploadFiles := func() error {
		return s.S3.UploadFiles(release, cfDomain, s.Scope.BucketName(), privateKey)
	}
	err = backoff.Retry(uploadFiles, b)
	if err != nil {
		s.Scope.Logger.Error(err, "failed to upload files")
		return err
	}

	// only restrict access when IRSA is used via Cloudfront in v18
	if key.IsV18Release(release) {
		uploadPolicy := func() error { return s.S3.UpdatePolicy(s.Scope.BucketName(), cfOaiId) }
		err = backoff.Retry(uploadPolicy, b)
		if err != nil {
			s.Scope.Logger.Error(err, "failed to upload policy")
			return err
		}
		s.Scope.Logger.Info("Restricted access to allow Cloudfront reaching S3 bucket", s.Scope.BucketName())
	}

	createOIDCProvider := func() error {
		return s.IAM.CreateOIDCProvider(release, cfDomain, s.Scope.BucketName(), s.Scope.Region())
	}
	err = backoff.Retry(createOIDCProvider, b)
	if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger.Error(err, "failed to create OIDC provider")
		return err
	} else {
		s.Scope.Info("Finished reconciling OIDC provider resource.")
	}

	err = s.IAM.CreateOIDCTags(s.Scope.AccountID(), s.Scope.BucketName(), s.Scope.Region(), customerTags)
	if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger.Error(err, "failed to create tags")
		return err
	}
	s.Scope.Logger.Info("Created tags for OIDC provider")

	oidcTags, err := s.IAM.ListCustomerOIDCTags(s.Scope.AccountID(), s.Scope.BucketName(), s.Scope.Region())
	if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger.Error(err, "failed to list OIDC provider tags")
		return err
	}

	if diff := util.MapsDiff(customerTags, oidcTags); diff != nil {
		s.Scope.Logger.Info("Cluster tags differ from current OIDC tags")
		if err := s.IAM.RemoveOIDCTags(s.Scope.AccountID(), s.Scope.BucketName(), s.Scope.Region(), diff); err != nil {
			ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
			s.Scope.Logger.Error(err, "failed to remove tags")
			return microerror.Mask(err)
		}
		s.Scope.Logger.Info("Removed tags for OIDC provider")
	}

	ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Set(0)
	s.Scope.Logger.Info("Reconciled all resources.")
	return nil
}

func (s *IRSAService) Delete(ctx context.Context) error {
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

	cfConfig := &v1.ConfigMap{}
	err = s.Client.Get(ctx, types.NamespacedName{Namespace: s.Scope.ClusterNamespace(), Name: s.Scope.ConfigName()}, cfConfig)
	if apierrors.IsNotFound(err) {
		s.Scope.Logger.Error(err, "failed to get configmap for OIDC cloudfront")
		return err
	} else if err != nil {
		return err
	}

	cfDomain := cfConfig.Data["domain"]
	release, _ := semver.New(s.Scope.ReleaseVersion())

	err = s.IAM.DeleteOIDCProvider(release, cfDomain, s.Scope.AccountID(), s.Scope.BucketName(), s.Scope.Region())
	if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger.Error(err, "failed to delete OIDC provider")
		return err
	}

	oidcSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: s.Scope.ClusterNamespace(),
			Name:      s.Scope.SecretName(),
		},
	}
	err = s.Client.Delete(ctx, oidcSecret, &client.DeleteOptions{PropagationPolicy: toDeletePropagation(metav1.DeletePropagationForeground)})
	if apierrors.IsNotFound(err) {
		// OIDC secret is already deleted
		// fall through
	} else if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger.Error(err, "failed to delete OIDC service account secret for cluster")
		return err
	}

	cfDistributionId := cfConfig.Data["distributionId"]
	cfOriginAccessIdentityId := cfConfig.Data["originAccessIdentityId"]

	err = s.Cloudfront.DisableDistribution(cfDistributionId)
	if err != nil {
		s.Scope.Logger.Error(err, "failed to disable cloudfront distribution for cluster")
		return err
	}

	deleteDistribution := func() error {
		err = s.Cloudfront.DeleteDistribution(cfDistributionId)
		if err != nil {
			s.Scope.Logger.Error(err, "failed to delete cloudfront distribution for cluster")
			return err
		}
		return nil
	}

	backoff.Retry(deleteDistribution, backoff.NewMaxRetries(30, 1*time.Minute))

	err = s.Cloudfront.DeleteOriginAccessIdentity(cfOriginAccessIdentityId)
	if err != nil {
		s.Scope.Logger.Error(err, "failed to delete cloudfront origin access identity for cluster")
		return err
	}

	cloudfrontConfigMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: s.Scope.ClusterNamespace(),
			Name:      s.Scope.ConfigName(),
		},
	}
	err = s.Client.Delete(ctx, cloudfrontConfigMap, &client.DeleteOptions{PropagationPolicy: toDeletePropagation(metav1.DeletePropagationForeground)})
	if apierrors.IsNotFound(err) {
		// OIDC cloudfront config map is already deleted
		// fall through
	} else if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger.Error(err, "failed to delete OIDC cloudfront config map for cluster")
		return microerror.Mask(err)
	}

	ctrlmetrics.Errors.DeleteLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace())
	s.Scope.Logger.Info("All IRSA resource have been successfully deleted.")

	return nil
}

func toDeletePropagation(v metav1.DeletionPropagation) *metav1.DeletionPropagation {
	return &v
}

func getCustomerTags(cluster *capi.Cluster) map[string]string {
	customerTags := make(map[string]string)

	for k, v := range cluster.Labels {
		if strings.HasPrefix(k, key.CustomerTagLabel) {
			customerTags[strings.Replace(k, key.CustomerTagLabel, "", 1)] = v
		}
	}
	return customerTags
}
