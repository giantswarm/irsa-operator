package irsa

import (
	"context"

	"github.com/giantswarm/irsa-operator/pkg/aws/scope"
	"github.com/giantswarm/irsa-operator/pkg/aws/services/iam"
	"github.com/giantswarm/irsa-operator/pkg/aws/services/s3"
	"github.com/giantswarm/irsa-operator/pkg/files"
	"github.com/giantswarm/microerror"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	oidcSecret := &v1.Secret{}
	err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Scope.ClusterNamespace(), Name: s.Scope.SecretName()}, oidcSecret)
	if apierrors.IsNotFound(err) {
		// create new OIDC service account secret
		err := files.Generate(s.Scope.BucketName(), s.Scope.Region())
		if err != nil {
			s.Scope.Logger.Error(err, "failed to generate files for cluster")
			return microerror.Mask(err)
		}

		privateSignerKey, err := files.ReadFile(s.Scope.BucketName(), files.PrivateSignerKeyFilename)
		if err != nil {
			s.Scope.Logger.Error(err, "failed to read private signer key file for cluster")
			return microerror.Mask(err)

		}
		publicSignerKey, err := files.ReadFile(s.Scope.BucketName(), files.PublicSignerKeyFilename)
		if err != nil {
			s.Scope.Logger.Error(err, "failed to read public signer key file for cluster")
			return microerror.Mask(err)

		}

		oidcSecret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      s.Scope.SecretName(),
				Namespace: s.Scope.ClusterNamespace(),
			},
			StringData: map[string]string{
				"service-account-v2.key": string(privateSignerKey),
				"service-account-v2.pub": string(publicSignerKey),
			},
			Type: v1.SecretTypeOpaque,
		}

		if err := s.Client.Create(ctx, oidcSecret); err != nil {
			s.Scope.Logger.Error(err, "failed to create OIDC service account secret for cluster")
			return microerror.Mask(err)
		}
		err = s.S3.CreateBucket(s.Scope.BucketName())
		if err != nil {
			s.Scope.Logger.Error(err, "failed to create bucket")
			return microerror.Mask(err)
		}
		err = s.S3.UploadFiles(s.Scope.BucketName())
		if err != nil {
			s.Scope.Logger.Error(err, "failed to upload files")
			return microerror.Mask(err)
		}
		err = s.IAM.CreateOIDCProvider(s.Scope.BucketName(), s.Scope.Region())
		if err != nil {
			s.Scope.Logger.Error(err, "failed to create OIDC provider")
			return microerror.Mask(err)
		}
		err = files.Delete(s.Scope.BucketName())
		if err != nil {
			s.Scope.Logger.Error(err, "failed to delete temp files")
			return microerror.Mask(err)
		}
	} else if err != nil {
		s.Scope.Logger.Error(err, "failed to get OIDC service account secret for cluster")
		return microerror.Mask(err)
	}
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
	return nil
}

func toDeletePropagation(v metav1.DeletionPropagation) *metav1.DeletionPropagation {
	return &v
}
