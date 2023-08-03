package eks

import (
	"github.com/aws/aws-sdk-go/service/eks/eksiface"

	"github.com/giantswarm/irsa-operator/pkg/aws/scope"
)

// Service holds a collection of interfaces.
type Service struct {
	scope  scope.EKSScope
	Client eksiface.EKSAPI
}

// NewService returns a new service given the S3 api client.
func NewService(clusterScope scope.IAMScope) *Service {
	return &Service{
		scope:  clusterScope,
		Client: scope.NewEKSClient(clusterScope, clusterScope.ARN(), clusterScope.Cluster()),
	}
}
