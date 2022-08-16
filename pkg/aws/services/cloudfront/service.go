package cloudfront

import (
	"github.com/aws/aws-sdk-go/service/cloudfront/cloudfrontiface"

	"github.com/giantswarm/irsa-operator/pkg/aws/scope"
)

// Service holds a collection of interfaces.
type Service struct {
	scope  scope.CloudfrontScope
	Client cloudfrontiface.CloudFrontAPI
}

// NewService returns a new service given the Cloudfront api client.
func NewService(clusterScope scope.IAMScope) *Service {
	return &Service{
		scope:  clusterScope,
		Client: scope.NewCloudfrontClient(clusterScope, clusterScope.ARN(), clusterScope.Cluster()),
	}
}
