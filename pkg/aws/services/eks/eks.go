package eks

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/giantswarm/microerror"
)

// GetEKSOpenIDConnectProviderURL fetches OpenID Connect provider URL for the EKS cluster
func (s *Service) GetEKSOpenIDConnectProviderURL(clusterName string) (string, error) {
	i := &eks.DescribeClusterInput{
		Name: aws.String(clusterName),
	}
	cluster, err := s.Client.DescribeCluster(i)
	if err != nil {
		return "", microerror.Mask(err)
	}
	return *cluster.Cluster.Identity.Oidc.Issuer, nil
}
