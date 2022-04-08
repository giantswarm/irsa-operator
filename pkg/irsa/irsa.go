package irsa

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
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
	"github.com/giantswarm/irsa-operator/pkg/aws/services/iam"
	"github.com/giantswarm/irsa-operator/pkg/aws/services/s3"
	"github.com/giantswarm/irsa-operator/pkg/key"
	"github.com/giantswarm/irsa-operator/pkg/pkcs"
)

type IRSAService struct {
	Client client.Client
	Scope  *scope.ClusterScope

	S3  *s3.Service
	IAM *iam.Service
}

func New(scope *scope.ClusterScope, client client.Client) *IRSAService {
	return &IRSAService{
		Scope:  scope,
		Client: client,

		IAM: iam.NewService(scope),
		S3:  s3.NewService(scope),
	}
}

func (s *IRSAService) Reconcile(ctx context.Context) error {
	var key *rsa.PrivateKey

	s.Scope.Info("Reconciling AWSCluster CR for IRSA")
	oidcSecret := &v1.Secret{}
	err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Scope.ClusterNamespace(), Name: s.Scope.SecretName()}, oidcSecret)
	if apierrors.IsNotFound(err) {
		// create new OIDC service account secret
		privateSignerKey, publicSignerKey, pkey, err := pkcs.GenerateKeys()
		if err != nil {
			return microerror.Mask(err)
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
			s.Scope.Logger.Error(err, "failed to create OIDC service account secret for cluster")
			return microerror.Mask(err)
		}
		s.Scope.Logger.Info("Created secret signer keys in k8s")

		key = pkey
	} else if err == nil {
		// if secret already exists, parse the private key
		privBytes := oidcSecret.Data["key"]
		block, _ := pem.Decode(privBytes)
		pkey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return microerror.Mask(err)
		}
		key = pkey.(*rsa.PrivateKey)
	} else {
		return microerror.Mask(err)
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
			s.Scope.Logger.Error(err, "failed to create bucket")
			return microerror.Mask(err)
		}
		s.Scope.Logger.Info("Created S3 bucket", s.Scope.BucketName())

	} else {
		s.Scope.Logger.Info("S3 bucket already exists, skipping creation", s.Scope.BucketName())
	}

	err = s.S3.EncryptBucket(s.Scope.BucketName())
	if err != nil {
		s.Scope.Logger.Error(err, "failed to encrypt bucket")
		return microerror.Mask(err)
	}
	s.Scope.Logger.Info("Encrypted S3 bucket", s.Scope.BucketName())

	// Fetch custom tags from Cluster CR
	cluster := &capi.Cluster{}
	err = s.Client.Get(ctx, types.NamespacedName{Namespace: s.Scope.ClusterNamespace(), Name: s.Scope.ClusterName()}, cluster)
	if apierrors.IsNotFound(err) {
		// fallthrough
	} else if err != nil {
		return microerror.Mask(err)
	}

	customerTags := getCustomerTags(cluster)

	err = s.S3.CreateTags(s.Scope.BucketName(), customerTags)
	if err != nil {
		s.Scope.Logger.Error(err, "failed to create tags")
		return microerror.Mask(err)
	}
	s.Scope.Logger.Info("Created tags for S3 bucket", s.Scope.BucketName())

	uploadFiles := func() error { return s.S3.UploadFiles(s.Scope.BucketName(), key) }
	err = backoff.Retry(uploadFiles, b)
	if err != nil {
		s.Scope.Logger.Error(err, "failed to upload files")
		return microerror.Mask(err)
	}

	createOIDCProvider := func() error { return s.IAM.CreateOIDCProvider(s.Scope.BucketName(), s.Scope.Region()) }
	err = backoff.Retry(createOIDCProvider, b)
	if err != nil {
		s.Scope.Logger.Error(err, "failed to create OIDC provider")
		return microerror.Mask(err)
	} else {
		s.Scope.Info("Finished reconciling OIDC provider resource.")
	}

	err = s.IAM.CreateOIDCTags(s.Scope.AccountID(), s.Scope.BucketName(), s.Scope.Region(), customerTags)
	if err != nil {
		s.Scope.Logger.Error(err, "failed to create tags")
		return microerror.Mask(err)
	}
	s.Scope.Logger.Info("Created tags for OIDC", s.Scope.BucketName())

	s.Scope.Logger.Info("Reconciled all resources.")
	return nil
}

func (s *IRSAService) Delete(ctx context.Context) error {
	err := s.S3.DeleteFiles(s.Scope.BucketName())
	if err != nil {
		s.Scope.Logger.Error(err, "failed to delete S3 files")
		return microerror.Mask(err)
	}
	err = s.S3.DeleteBucket(s.Scope.BucketName())
	if err != nil {
		s.Scope.Logger.Error(err, "failed to delete S3 bucket")
		return microerror.Mask(err)
	}
	err = s.IAM.DeleteOIDCProvider(s.Scope.AccountID(), s.Scope.BucketName(), s.Scope.Region())
	if err != nil {
		s.Scope.Logger.Error(err, "failed to delete OIDC provider")
		return microerror.Mask(err)
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
		return nil
	} else if err != nil {
		s.Scope.Logger.Error(err, "failed to delete OIDC service account secret for cluster")
		return microerror.Mask(err)
	}

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
