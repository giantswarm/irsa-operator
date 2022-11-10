package acm

import (
	"github.com/aws/aws-sdk-go/service/acm/acmiface"

	"github.com/giantswarm/irsa-operator/pkg/aws/scope"
)

// Service holds a collection of interfaces.
type Service struct {
	scope  scope.ACMScope
	Client acmiface.ACMAPI
}

// NewService returns a new service given the Cloudfront api client.
func NewService(clusterScope scope.IAMScope) *Service {
	return &Service{
		scope:  clusterScope,
		Client: scope.NewACMClient(clusterScope, clusterScope.ARN(), clusterScope.Cluster()),
	}
}
