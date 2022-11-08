package cloudfront

import (
	"fmt"
	"reflect"
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

func (s *Service) CreateDistribution(config DistributionConfig) (*Distribution, error) {
	s.scope.Info("Ensuring cloudfront distribution")

	// Check if distribution already exists.
	d, err := s.findDistribution()
	if err != nil {
		s.scope.Error(err, "Error checking if cloudfront distribution already exists")
		return nil, err
	}

	var existing *cloudfront.Distribution
	var etag *string
	distributionNeedsUpdate := false
	tagsToBeAdded := map[string]string{}
	tagsToBeRemoved := make([]string, 0)

	if d != nil {
		s.scope.Info("Cloudfront distribution already exists")

		// Check if distribution is up to date.
		result, err := s.Client.GetDistribution(&cloudfront.GetDistributionInput{Id: aws.String(d.DistributionId)})
		if err != nil {
			s.scope.Error(err, "Error checking if cloudfront distribution is up to date")
			return nil, err
		}

		existing = result.Distribution
		etag = result.ETag

		tags, err := s.Client.ListTagsForResource(&cloudfront.ListTagsForResourceInput{
			Resource: existing.ARN,
		})
		if err != nil {
			s.scope.Error(err, "Error listing tags")
			return nil, err
		}

		distributionNeedsUpdate = s.distributionNeedsUpdate(result.Distribution, config)
		tagsToBeAdded, tagsToBeRemoved = tagsNeedUpdating(tags.Tags, s.internalTags(), config)

		if !distributionNeedsUpdate && len(tagsToBeAdded)+len(tagsToBeRemoved) == 0 {
			s.scope.Info("Distribution is up to date")
			return d, nil
		}
	}

	oaiId, err := s.CreateOriginAccessIdentity()
	if err != nil {
		s.scope.Error(err, "Error creating cloudfront origin access identity")
		return nil, err
	}

	distributionConfigWithTags := &cloudfront.DistributionConfigWithTags{
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
		},
		Tags: &cloudfront.Tags{
			Items: []*cloudfront.Tag{},
		},
	}

	if config.CertificateArn != "" {
		distributionConfigWithTags.DistributionConfig.ViewerCertificate = &cloudfront.ViewerCertificate{
			ACMCertificateArn:      aws.String(config.CertificateArn),
			MinimumProtocolVersion: aws.String(cloudfront.MinimumProtocolVersionTlsv122021),
			SSLSupportMethod:       aws.String(cloudfront.SSLSupportMethodSniOnly),
		}
	}

	for k, v := range s.internalTags() {
		tag := &cloudfront.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		}
		distributionConfigWithTags.Tags.Items = append(distributionConfigWithTags.Tags.Items, tag)
	}

	for k, v := range config.CustomerTags {
		tag := &cloudfront.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		}
		distributionConfigWithTags.Tags.Items = append(distributionConfigWithTags.Tags.Items, tag)
	}

	i := &cloudfront.CreateDistributionWithTagsInput{DistributionConfigWithTags: distributionConfigWithTags}

	if existing != nil {
		if distributionNeedsUpdate {
			// Update existing distribution.

			s.scope.Info("Updating distribution")

			// Take the existing distributionConfig (with all defaulting happened on AWS side) and override with our desired settings.
			dc := existing.DistributionConfig
			dc.Aliases = distributionConfigWithTags.DistributionConfig.Aliases
			dc.ViewerCertificate = distributionConfigWithTags.DistributionConfig.ViewerCertificate

			o, err := s.Client.UpdateDistribution(&cloudfront.UpdateDistributionInput{
				DistributionConfig: dc,
				Id:                 aws.String(d.DistributionId),
				IfMatch:            etag,
			})
			if err != nil {
				s.scope.Error(err, "Error updating cloudfront distribution")
				return nil, err
			}

			s.scope.Info("Updated distribution")

			return &Distribution{ARN: *o.Distribution.ARN, DistributionId: *o.Distribution.Id, Domain: *o.Distribution.DomainName, OriginAccessIdentityId: oaiId}, nil
		}

		if len(tagsToBeAdded) > 0 {
			s.scope.Info(fmt.Sprintf("Adding %d tags", len(tagsToBeAdded)))
			_, err := s.Client.TagResource(&cloudfront.TagResourceInput{
				Resource: existing.ARN,
				Tags:     distributionConfigWithTags.Tags,
			})
			if err != nil {
				s.scope.Error(err, "Error adding cloudfront tags")
				return nil, err
			}

			s.scope.Info("Added tags")
		}

		if len(tagsToBeRemoved) > 0 {
			keys := make([]*string, 0)
			for _, k := range tagsToBeRemoved {
				keys = append(keys, aws.String(k))
			}
			s.scope.Info(fmt.Sprintf("Deleting %d tags", len(keys)))
			_, err := s.Client.UntagResource(&cloudfront.UntagResourceInput{
				Resource: existing.ARN,
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

		return &Distribution{ARN: *existing.ARN, DistributionId: *existing.Id, Domain: *existing.DomainName, OriginAccessIdentityId: oaiId}, nil

	} else {
		// Create new distribution.
		o, err := s.Client.CreateDistributionWithTags(i)
		if err != nil {
			s.scope.Error(err, "Error creating cloudfront distribution")
			return nil, err
		}
		s.scope.Info("Created cloudfront distribution")

		return &Distribution{ARN: *o.Distribution.ARN, DistributionId: *o.Distribution.Id, Domain: *o.Distribution.DomainName, OriginAccessIdentityId: oaiId}, nil
	}
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

// distributionNeedsUpdate compares the cloud front distribution agains the desired settings and returns true
// when the distribution needs to be updated.
func (s *Service) distributionNeedsUpdate(distribution *cloudfront.Distribution, config DistributionConfig) bool {
	changed := false
	if (distribution.DistributionConfig.Aliases == nil && config.Aliases != nil) ||
		(distribution.DistributionConfig.Aliases != nil && distribution.DistributionConfig.Aliases.Items != nil && config.Aliases == nil) ||
		(distribution.DistributionConfig.Aliases != nil && distribution.DistributionConfig.Aliases.Items != nil && config.Aliases != nil && len(distribution.DistributionConfig.Aliases.Items) != len(config.Aliases)) {
		s.scope.Info("Distribution Aliases need to be updated")
		changed = true
	} else {
		// desired and current Aliases are slices with the same size, but might still be different.
		currentAliases := make([]string, 0)
		desiredAliases := make([]string, 0)

		if distribution.DistributionConfig.Aliases != nil {
			for _, alias := range distribution.DistributionConfig.Aliases.Items {
				currentAliases = append(currentAliases, *alias)
			}
		}

		for _, alias := range config.Aliases {
			desiredAliases = append(desiredAliases, *alias)
		}

		if !reflect.DeepEqual(currentAliases, desiredAliases) {
			s.scope.Info("Distribution Aliases need to be updated")
			changed = true
		}
	}

	if (distribution.DistributionConfig.ViewerCertificate == nil && config.CertificateArn != "") ||
		(distribution.DistributionConfig.ViewerCertificate != nil && distribution.DistributionConfig.ViewerCertificate.ACMCertificateArn == nil && config.CertificateArn != "") ||
		(distribution.DistributionConfig.ViewerCertificate != nil && distribution.DistributionConfig.ViewerCertificate.ACMCertificateArn != nil && *distribution.DistributionConfig.ViewerCertificate.ACMCertificateArn != config.CertificateArn) {
		s.scope.Info("Distribution viewer certificate needs to be updated")
		changed = true
	}

	return changed
}

// tagsNeedUpdating compares current tags in the cloudfront distribution with default tags and customer tags
// and returns two map with tags to be added and tags to be removed
func tagsNeedUpdating(tags *cloudfront.Tags, internalTags map[string]string, config DistributionConfig) (tagsToBeAdded map[string]string, tagsToBeRemoved []string) {
	tagsToBeAdded = make(map[string]string, 0)
	tagsToBeRemoved = make([]string, 0)

	desiredTags := map[string]string{}
	currentTags := map[string]string{}

	if tags != nil {
		for _, tag := range tags.Items {
			currentTags[*tag.Key] = *tag.Value
		}
	}

	for k, v := range config.CustomerTags {
		desiredTags[k] = v
	}
	for k, v := range internalTags {
		desiredTags[k] = v
	}

	for k, v := range desiredTags {
		if val, found := currentTags[k]; !found || val != v {
			tagsToBeAdded[k] = v
		}
	}

	for k := range currentTags {
		if _, found := desiredTags[k]; !found {
			tagsToBeRemoved = append(tagsToBeRemoved, k)
		}
	}

	return
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
