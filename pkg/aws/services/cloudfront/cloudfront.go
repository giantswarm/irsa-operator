package cloudfront

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/giantswarm/microerror"

	"github.com/giantswarm/irsa-operator/pkg/key"
	"github.com/giantswarm/irsa-operator/pkg/util"
)

type Distribution struct {
	ARN                    string
	DistributionId         string
	Domain                 string
	OriginAccessIdentityId string
}

type DistributionConfig struct {
	Aliases        []*string
	CertificateArn string
	CustomerTags   map[string]string
}

func (s *Service) CreateOriginAccessIdentity() (string, error) {
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
	s.scope.Info("Created cloudfront origin access identity")

	return *o.CloudFrontOriginAccessIdentity.Id, nil
}

func (s *Service) EnsureDistribution(config DistributionConfig) (*Distribution, error) {
	s.scope.Info("Ensuring cloudfront distribution")

	// Check if distribution already exists.
	d, err := s.findDistribution()
	if err != nil {
		s.scope.Error(err, "Error checking if cloudfront distribution already exists")
		return nil, err
	}

	diff, err := s.checkDiff(d, config)
	if err != nil {
		s.scope.Error(err, "Error checking if cloudfront distribution needs to be updated")
		return nil, err
	}

	if diff.IsUpToDate() {
		s.scope.Info("Cloudfront distribution is up to date")
		return d, nil
	}

	oaiId, err := s.CreateOriginAccessIdentity()
	if err != nil {
		s.scope.Error(err, "Error creating cloudfront origin access identity")
		return nil, err
	}

	i := &cloudfront.CreateDistributionWithTagsInput{
		DistributionConfigWithTags: &cloudfront.DistributionConfigWithTags{
			DistributionConfig: &cloudfront.DistributionConfig{
				Aliases: &cloudfront.Aliases{
					Items:    config.Aliases,
					Quantity: aws.Int64(int64(len(config.Aliases))),
				},
				CallerReference: aws.String(fmt.Sprintf("distribution-cluster-%s", s.scope.ClusterName())),
				Comment:         aws.String(fmt.Sprintf("Created by irsa-operator for cluster %s", s.scope.ClusterName())),
				DefaultCacheBehavior: &cloudfront.DefaultCacheBehavior{
					// AWS managed cache policy id, caching is disabled for the distribution.
					CachePolicyId:        aws.String("4135ea2d-6df8-44a3-9df3-4b5a84be39ad"),
					TargetOriginId:       aws.String(fmt.Sprintf("%s.s3.%s.%s", s.scope.BucketName(), s.scope.Region(), key.AWSEndpoint(s.scope.Region()))),
					ViewerProtocolPolicy: aws.String("redirect-to-https"),
				},
				Enabled: aws.Bool(true),
				Origins: &cloudfront.Origins{
					Items: []*cloudfront.Origin{
						{
							Id:         aws.String(fmt.Sprintf("%s.s3.%s.%s", s.scope.BucketName(), s.scope.Region(), key.AWSEndpoint(s.scope.Region()))),
							DomainName: aws.String(fmt.Sprintf("%s.s3.%s.%s", s.scope.BucketName(), s.scope.Region(), key.AWSEndpoint(s.scope.Region()))),
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
				Restrictions: &cloudfront.Restrictions{
					GeoRestriction: &cloudfront.GeoRestriction{
						RestrictionType: aws.String("none"),
						Quantity:        aws.Int64(0),
					},
				},
				ViewerCertificate: &cloudfront.ViewerCertificate{
					MinimumProtocolVersion: aws.String(cloudfront.MinimumProtocolVersionTlsv122021),
					SSLSupportMethod:       aws.String(cloudfront.SSLSupportMethodSniOnly),
				},
			},
			Tags: &cloudfront.Tags{
				Items: []*cloudfront.Tag{},
			},
		},
	}

	if config.CertificateArn == "" {
		i.DistributionConfigWithTags.DistributionConfig.ViewerCertificate.ACMCertificateArn = nil
		i.DistributionConfigWithTags.DistributionConfig.ViewerCertificate.Certificate = nil
		i.DistributionConfigWithTags.DistributionConfig.ViewerCertificate.CertificateSource = nil
	} else {
		i.DistributionConfigWithTags.DistributionConfig.ViewerCertificate.SetACMCertificateArn(config.CertificateArn)
	}

	// Add internal and customer tags.
	{
		for k, v := range s.internalTags() {
			tag := &cloudfront.Tag{
				Key:   aws.String(k),
				Value: aws.String(v),
			}
			i.DistributionConfigWithTags.Tags.Items = append(i.DistributionConfigWithTags.Tags.Items, tag)
		}

		for k, v := range config.CustomerTags {
			tag := &cloudfront.Tag{
				Key:   aws.String(k),
				Value: aws.String(v),
			}
			i.DistributionConfigWithTags.Tags.Items = append(i.DistributionConfigWithTags.Tags.Items, tag)
		}
	}

	if diff.NeedsCreate {
		// Create new distribution.
		o, err := s.Client.CreateDistributionWithTags(i)
		if err != nil {
			s.scope.Error(err, "Error creating cloudfront distribution")
			return nil, err
		}
		s.scope.Info("Created cloudfront distribution")

		return &Distribution{ARN: *o.Distribution.ARN, DistributionId: *o.Distribution.Id, Domain: *o.Distribution.DomainName, OriginAccessIdentityId: oaiId}, nil
	} else if diff.NeedsUpdate {
		// Update existing distribution.

		s.scope.Info("Updating distribution")

		// Take the existing distributionConfig (with all defaulting happened on AWS side) and override with our desired settings.
		dc := diff.Existing.DistributionConfig
		dc.Aliases = i.DistributionConfigWithTags.DistributionConfig.Aliases
		dc.ViewerCertificate = i.DistributionConfigWithTags.DistributionConfig.ViewerCertificate

		_, err := s.Client.UpdateDistribution(&cloudfront.UpdateDistributionInput{
			DistributionConfig: dc,
			Id:                 aws.String(d.DistributionId),
			IfMatch:            diff.ETag,
		})
		if err != nil {
			s.scope.Error(err, "Error updating cloudfront distribution")
			return nil, err
		}

		s.scope.Info("Updated distribution")
	}

	if len(diff.TagsToBeAdded) > 0 {
		s.scope.Info(fmt.Sprintf("Adding %d tags", len(diff.TagsToBeAdded)))
		_, err := s.Client.TagResource(&cloudfront.TagResourceInput{
			Resource: diff.Existing.ARN,
			Tags:     i.DistributionConfigWithTags.Tags,
		})
		if err != nil {
			s.scope.Error(err, "Error adding cloudfront tags")
			return nil, err
		}

		s.scope.Info("Added tags")
	}

	if len(diff.TagsToBeRemoved) > 0 {
		keys := make([]*string, 0)
		for _, k := range diff.TagsToBeRemoved {
			keys = append(keys, aws.String(k))
		}
		s.scope.Info(fmt.Sprintf("Deleting %d tags", len(keys)))
		_, err := s.Client.UntagResource(&cloudfront.UntagResourceInput{
			Resource: diff.Existing.ARN,
			TagKeys: &cloudfront.TagKeys{
				Items: keys,
			},
		})
		if err != nil {
			s.scope.Error(err, "Error deleting cloudfront tags")
			return nil, err
		}

		s.scope.Info("Tags deleted")
	}

	return &Distribution{ARN: *diff.Existing.ARN, DistributionId: *diff.Existing.Id, Domain: *diff.Existing.DomainName, OriginAccessIdentityId: oaiId}, nil
}

func (s *Service) findDistribution() (*Distribution, error) {
	// Check if distribution already exists
	var err error
	var output *cloudfront.ListDistributionsOutput

	// Marker is the way AWS API performs pagination over results.
	// If Marker is not nil, there is another page of results to be requested.
	// If output is nil, means we have to request the very first page of results.
	for output == nil || output.DistributionList.Marker != nil {
		var marker *string
		if output != nil && output.DistributionList != nil {
			marker = output.DistributionList.Marker
		}
		output, err = s.Client.ListDistributions(&cloudfront.ListDistributionsInput{Marker: marker})
		if err != nil {
			return nil, microerror.Mask(err)
		}

		if len(output.DistributionList.Items) == 0 {
			return nil, nil
		}

		for _, d := range output.DistributionList.Items {
			// There are no tags in this API response, so we have to match on the Comment :(
			if *d.Comment == key.CloudFrontDistributionComment(s.scope.ClusterName()) {
				// This is something like origin-access-identity/cloudfront/E2IB68Y7SJQAKJ
				fullId := *d.Origins.Items[0].S3OriginConfig.OriginAccessIdentity

				tokens := strings.Split(fullId, "/")
				if len(tokens) != 3 {
					s.scope.Error(invalidOriginAccessIdentity, "Unexpected format for the Cloud Front S3OriginConfig OriginAccessIdentity field")
					return nil, microerror.Mask(err)
				}

				// We just want the final ID
				oaID := tokens[2]

				return &Distribution{
					ARN:                    *d.ARN,
					DistributionId:         *d.Id,
					Domain:                 *d.DomainName,
					OriginAccessIdentityId: oaID,
				}, nil
			}
		}
	}

	return nil, nil
}

func (s *Service) internalTags() map[string]string {
	return map[string]string{
		key.S3TagOrganization: util.RemoveOrg(s.scope.ClusterNamespace()),
		key.S3TagCluster:      s.scope.ClusterName(),
		fmt.Sprintf(key.S3TagCloudProvider, s.scope.ClusterName()): "owned",
		key.S3TagInstallation: s.scope.Installation(),
	}
}

func (s *Service) DisableDistribution(distributionId string) error {
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
	s.scope.Info("Disabled cloudfront distribution")
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
	_, eTag, err := s.getDistribution(distributionId)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case cloudfront.ErrCodeDistributionNotDisabled:
				s.scope.Info("Cloudfront distribution is not disabled yet, waiting ...")
				return err
			case cloudfront.ErrCodeNoSuchDistribution:
				s.scope.Info("Cloudfront distribution no longer exists, skipping deletion")
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
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case cloudfront.ErrCodeDistributionNotDisabled:
				s.scope.Info("Cloudfront distribution is not disabled yet, waiting ...")
				return err
			}
		}
		s.scope.Error(err, "Error deleting cloudfront distribution")
		return err
	}
	s.scope.Info("Deleted cloudfront distribution")
	return nil
}

func (s *Service) DeleteOriginAccessIdentity(oaiId string) error {
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
	s.scope.Info("Deleted cloudfront origin access identity")
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
