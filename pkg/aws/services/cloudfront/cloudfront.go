package cloudfront

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/cloudfront"

	"github.com/giantswarm/irsa-operator/pkg/key"
	"github.com/giantswarm/irsa-operator/pkg/util"
)

type Distribution struct {
	ARN                    string
	DistributionId         string
	Domain                 string
	OriginAccessIdentityId string
}

func (s *Service) CreateOriginAccessIdentity() (string, error) {
	s.scope.Info("Creating cloudfront origin access identity")
	i := &cloudfront.CreateCloudFrontOriginAccessIdentityInput{
		CloudFrontOriginAccessIdentityConfig: &cloudfront.OriginAccessIdentityConfig{
			CallerReference: aws.String(fmt.Sprintf("access-identity-cluster-%s", s.scope.ClusterName())),
			Comment:         aws.String(fmt.Sprintf("Created by irsa-operator for cluster %s", s.scope.ClusterName())),
		},
	}
	o, err := s.Client.CreateCloudFrontOriginAccessIdentity(i)
	if err != nil {
		s.scope.Error(err, "Error creating cloudfront origin access identity")
		return "", err
	}

	return *o.CloudFrontOriginAccessIdentity.Id, nil
}

func (s *Service) CreateDistribution(accountID string) (*Distribution, error) {
	s.scope.Info("Creating cloudfront distribution")
	oaiId, err := s.CreateOriginAccessIdentity()
	if err != nil {
		s.scope.Error(err, "Error creating cloudfront origin access identity")
		return nil, err
	}
	i := &cloudfront.CreateDistributionWithTagsInput{
		DistributionConfigWithTags: &cloudfront.DistributionConfigWithTags{
			DistributionConfig: &cloudfront.DistributionConfig{
				Comment:         aws.String(fmt.Sprintf("Created by irsa-operator for cluster %s", s.scope.ClusterName())),
				CallerReference: aws.String(fmt.Sprintf("distribution-cluster-%s", s.scope.ClusterName())),
				DefaultCacheBehavior: &cloudfront.DefaultCacheBehavior{
					// Caching is disabled for the distribution.
					CachePolicyId:        aws.String("4135ea2d-6df8-44a3-9df3-4b5a84be39ad"),
					TargetOriginId:       aws.String(fmt.Sprintf("%s-g8s-%s-oidc-pod-identity.s3.%s.%s", accountID, s.scope.ClusterName(), s.scope.Region(), key.AWSEndpoint(s.scope.Region()))),
					ViewerProtocolPolicy: aws.String("redirect-to-https"),
				},
				Restrictions: &cloudfront.Restrictions{
					GeoRestriction: &cloudfront.GeoRestriction{
						RestrictionType: aws.String("none"),
						Quantity:        aws.Int64(0),
					},
				},
				Enabled:       aws.Bool(true),
				IsIPV6Enabled: aws.Bool(true),
				Origins: &cloudfront.Origins{
					Items: []*cloudfront.Origin{
						{
							Id:         aws.String(fmt.Sprintf("%s-g8s-%s-oidc-pod-identity.s3.%s.%s", accountID, s.scope.ClusterName(), s.scope.Region(), key.AWSEndpoint(s.scope.Region()))),
							DomainName: aws.String(fmt.Sprintf("%s-g8s-%s-oidc-pod-identity.s3.%s.%s", accountID, s.scope.ClusterName(), s.scope.Region(), key.AWSEndpoint(s.scope.Region()))),
							OriginShield: &cloudfront.OriginShield{
								Enabled: aws.Bool(false),
							},
							S3OriginConfig: &cloudfront.S3OriginConfig{
								OriginAccessIdentity: aws.String(fmt.Sprintf("origin-access-identity/cloudfront/%s", oaiId)),
							},
						},
					},
					Quantity: aws.Int64(1),
				},
			},
			Tags: &cloudfront.Tags{
				Items: []*cloudfront.Tag{
					{
						Key:   aws.String(key.S3TagOrganization),
						Value: aws.String(util.RemoveOrg(s.scope.ClusterNamespace())),
					},
					{
						Key:   aws.String(key.S3TagCluster),
						Value: aws.String(s.scope.ClusterName()),
					},
					{
						Key:   aws.String(fmt.Sprintf(key.S3TagCloudProvider, s.scope.ClusterName())),
						Value: aws.String("owned"),
					},
					{
						Key:   aws.String(key.S3TagInstallation),
						Value: aws.String(s.scope.Installation()),
					},
				},
			},
		},
	}
	o, err := s.Client.CreateDistributionWithTags(i)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case cloudfront.ErrCodeDistributionAlreadyExists:
				s.scope.Info("Cloudfront distribution already exists, ignoring creation")
				return nil, nil
			}
		}
		s.scope.Error(err, "Error creating cloudfront distribution")
		return nil, err
	}

	return &Distribution{ARN: *o.Distribution.ARN, DistributionId: *o.Distribution.Id, Domain: *o.Distribution.DomainName, OriginAccessIdentityId: oaiId}, nil
}

func (s *Service) DisableDistribution(distributionId string) error {
	s.scope.Info("Disabling cloudfront distribution")
	distributionConfig, eTag, err := s.getDistribution(distributionId)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case cloudfront.ErrCodeNoSuchDistribution:
				s.scope.Info("Cloudfront distibution no longer exists, skipping deletion")
				return nil
			}
		}
		return err
	}
	distributionConfig.SetEnabled(false)
	i := &cloudfront.UpdateDistributionInput{
		DistributionConfig: distributionConfig,
		Id:                 aws.String(distributionId),
		IfMatch:            eTag,
	}
	_, err = s.Client.UpdateDistribution(i)
	if err != nil {
		s.scope.Error(err, "Error disabling cloudfront distribution")
		return err
	}
	return nil
}

func (s *Service) getDistribution(distributionId string) (*cloudfront.DistributionConfig, *string, error) {
	i := &cloudfront.GetDistributionInput{
		Id: aws.String(distributionId),
	}

	o, err := s.Client.GetDistribution(i)
	if err != nil {
		return nil, nil, err
	}
	return o.Distribution.DistributionConfig, o.ETag, nil

}

func (s *Service) DeleteDistribution(distributionId string) error {
	s.scope.Info("Deleting cloudfront distribution")
	_, eTag, err := s.getDistribution(distributionId)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case cloudfront.ErrCodeNoSuchDistribution:
				s.scope.Info("Cloudfronr distribution no longer exists, skipping deletion")
				return nil
			}
		}
		return err
	}
	i := &cloudfront.DeleteDistributionInput{
		Id:      aws.String(distributionId),
		IfMatch: eTag,
	}
	_, err = s.Client.DeleteDistribution(i)
	if err != nil {
		s.scope.Error(err, "Error deleting cloudfront distribution")
		return err
	}
	return nil
}

func (s *Service) DeleteOriginAccessIdentity(oaiId string) error {
	s.scope.Info("Deleting cloudfront origin access identity")
	_, eTag, err := s.GetOriginAccessIdentity(oaiId)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case cloudfront.ErrCodeNoSuchCloudFrontOriginAccessIdentity:
				s.scope.Info("Origin access identity no longer exists, skipping deletion")
				return nil
			}
		}
		s.scope.Error(err, "Error getting cloudfront origin access identity")
		return err
	}
	i := &cloudfront.DeleteCloudFrontOriginAccessIdentityInput{
		Id:      aws.String(oaiId),
		IfMatch: eTag,
	}
	_, err = s.Client.DeleteCloudFrontOriginAccessIdentity(i)
	if err != nil {
		s.scope.Error(err, "Error deleting cloudfront origin access identity")
		return err
	}
	return nil
}

func (s *Service) GetOriginAccessIdentity(oaiId string) (*cloudfront.OriginAccessIdentity, *string, error) {
	i := &cloudfront.GetCloudFrontOriginAccessIdentityInput{
		Id: aws.String(oaiId),
	}
	o, err := s.Client.GetCloudFrontOriginAccessIdentity(i)
	if err != nil {
		return nil, nil, err
	}

	return o.CloudFrontOriginAccessIdentity, o.ETag, nil
}
