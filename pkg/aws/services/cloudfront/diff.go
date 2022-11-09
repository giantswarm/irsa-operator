package cloudfront

import (
	"reflect"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudfront"
)

type Diff struct {
	ETag            *string
	Existing        *cloudfront.Distribution
	NeedsUpdate     bool
	NeedsCreate     bool
	TagsToBeAdded   map[string]string
	TagsToBeRemoved []string
}

func (s *Service) checkDiff(d *Distribution, config DistributionConfig) (*Diff, error) {
	ret := &Diff{}

	if d == nil {
		ret.NeedsCreate = true
	} else {
		s.scope.Info("Cloudfront distribution already exists")

		// Check if distribution is up to date.
		result, err := s.Client.GetDistribution(&cloudfront.GetDistributionInput{Id: aws.String(d.DistributionId)})
		if err != nil {
			s.scope.Error(err, "Error checking if cloudfront distribution is up to date")
			return nil, err
		}

		ret.Existing = result.Distribution
		ret.ETag = result.ETag

		tags, err := s.Client.ListTagsForResource(&cloudfront.ListTagsForResourceInput{
			Resource: result.Distribution.ARN,
		})
		if err != nil {
			s.scope.Error(err, "Error listing tags")
			return nil, err
		}

		ret.NeedsUpdate = s.distributionNeedsUpdate(result.Distribution, config)
		tagsToBeAdded, tagsToBeRemoved := tagsNeedUpdating(tags.Tags, s.internalTags(), config)
		ret.TagsToBeAdded = tagsToBeAdded
		ret.TagsToBeRemoved = tagsToBeRemoved
	}

	return ret, nil
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
