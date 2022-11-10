package route53

import (
	"github.com/aws/aws-sdk-go/service/route53/route53iface"

	"github.com/giantswarm/irsa-operator/pkg/aws/scope"
)

// Service holds a collection of interfaces.
type Service struct {
	scope  scope.Route53Scope
	Client route53iface.Route53API
}

// NewService returns a new service given the Cloudfront api client.
func NewService(clusterScope scope.IAMScope) *Service {
	return &Service{
		scope:  clusterScope,
		Client: scope.NewRoute53Client(clusterScope, clusterScope.ARN(), clusterScope.Cluster()),
	}
}
