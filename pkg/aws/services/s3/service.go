package s3

import (
	"github.com/aws/aws-sdk-go/service/s3/s3iface"

	"github.com/giantswarm/irsa-operator/pkg/aws/scope"
)

// Service holds a collection of interfaces.
type Service struct {
	scope  scope.S3Scope
	Client s3iface.S3API
}

// NewService returns a new service given the S3 api client.
func NewService(clusterScope scope.S3Scope) *Service {
	return &Service{
		scope:  clusterScope,
		Client: scope.NewS3Client(clusterScope, clusterScope.ARN(), clusterScope.Cluster()),
	}
}
