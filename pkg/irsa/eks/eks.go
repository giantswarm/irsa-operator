package eks

import (
	"context"

	"github.com/giantswarm/microerror"
	"k8s.io/apimachinery/pkg/types"
	controlplanecapa "sigs.k8s.io/cluster-api-provider-aws/v2/controlplane/eks/api/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/irsa-operator/pkg/aws/scope"
	"github.com/giantswarm/irsa-operator/pkg/aws/services/eks"
	"github.com/giantswarm/irsa-operator/pkg/aws/services/iam"
	"github.com/giantswarm/irsa-operator/pkg/key"
	ctrlmetrics "github.com/giantswarm/irsa-operator/pkg/metrics"
)

type Service struct {
	Client client.Client
	Scope  *scope.ClusterScope

	IAM *iam.Service
	EKS *eks.Service
}

func New(scope *scope.ClusterScope, client client.Client) *Service {
	return &Service{
		Scope:  scope,
		Client: client,

		EKS: eks.NewService(scope),
		IAM: iam.NewService(scope),
	}
}
func (s *Service) Reconcile(ctx context.Context) error {
	s.Scope.Logger().Info("Reconciling AWSManagedCluster CR for IRSA")
	oidcURL, err := s.EKS.GetEKSOpenIDConnectProviderURL(s.Scope.ClusterName())
	if err != nil {
		s.Scope.Logger().Error(err, "failed to fetch EKS OIDC issuer URL")
		return microerror.Mask(err)
	}
	identityProviderURLs := []string{oidcURL}

	// Fetch custom tags from Cluster CR
	cluster := &controlplanecapa.AWSManagedControlPlane{}
	err = s.Client.Get(ctx, types.NamespacedName{Namespace: s.Scope.ClusterNamespace(), Name: s.Scope.ClusterName()}, cluster)
	if err != nil {
		return microerror.Mask(err)
	}

	err = s.IAM.EnsureOIDCProviders(identityProviderURLs, key.STSUrl(s.Scope.Region()), cluster.Spec.AdditionalTags)
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
	err := s.IAM.DeleteOIDCProviders()
	if err != nil {
		ctrlmetrics.Errors.WithLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace()).Inc()
		s.Scope.Logger().Error(err, "failed to delete OIDC provider")
		return err
	}

	ctrlmetrics.Errors.DeleteLabelValues(s.Scope.Installation(), s.Scope.AccountID(), s.Scope.ClusterName(), s.Scope.ClusterNamespace())
	s.Scope.Logger().Info("Finished deleting all resources.")

	return nil
}
