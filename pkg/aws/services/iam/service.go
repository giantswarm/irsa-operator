package iam

import (
	"github.com/aws/aws-sdk-go/service/iam/iamiface"

	"github.com/giantswarm/irsa-operator/pkg/aws/scope"
)

// Service holds a collection of interfaces.
type Service struct {
	scope  scope.IAMScope
	Client iamiface.IAMAPI
}

// NewService returns a new service given the S3 api client.
func NewService(clusterScope scope.IAMScope) *Service {
	return &Service{
		scope:  clusterScope,
		Client: scope.NewIAMClient(clusterScope, clusterScope.ARN(), clusterScope.InfraCluster()),
	}
}
