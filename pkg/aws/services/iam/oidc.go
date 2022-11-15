package iam

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/blang/semver"
	"github.com/giantswarm/microerror"
	"github.com/nhalstead/sprint"

	"github.com/giantswarm/irsa-operator/pkg/key"
	"github.com/giantswarm/irsa-operator/pkg/util"
	"github.com/giantswarm/irsa-operator/pkg/util/slicediff"
)

func (s *Service) EnsureOIDCProviders(identityProviderURLs []string, clientID string, customerTags map[string]string) error {
	providers, err := s.findOIDCProviders()
	if err != nil {
		return microerror.Mask(err)
	}

	thumbprints := make([]*string, 0)
	for _, identityProviderURL := range identityProviderURLs {
		tp, err := caThumbPrint(identityProviderURL)
		if err != nil {
			return err
		}

		thumbprints = append(thumbprints, &tp)
	}

	// Ensure there is one provider for each of the URLs
	for _, identityProviderURL := range identityProviderURLs {
		// Check if one of the providers is already using the right URL.
		found := false
		for arn, existing := range providers {
			if util.EnsureHTTPS(*existing.Url) == util.EnsureHTTPS(identityProviderURL) {
				found = true
				thumbprintsDiff := slicediff.DiffIgnoreCase(existing.ThumbprintList, thumbprints)
				clientidsDiff := slicediff.DiffIgnoreCase(existing.ClientIDList, []*string{&clientID})

				for _, add := range clientidsDiff.Added {
					s.scope.Info(fmt.Sprintf("Adding client id %s to OIDCProvider for URL %s", add, identityProviderURL))
					_, err = s.Client.AddClientIDToOpenIDConnectProvider(&iam.AddClientIDToOpenIDConnectProviderInput{
						ClientID:                 &add,
						OpenIDConnectProviderArn: &arn,
					})
					if err != nil {
						return microerror.Mask(err)
					}
					s.scope.Info(fmt.Sprintf("Added client id %s to OIDCProvider for URL %s", add, identityProviderURL))
				}
				for _, remove := range clientidsDiff.Removed {
					s.scope.Info(fmt.Sprintf("Removing client id %s to OIDCProvider for URL %s", remove, identityProviderURL))
					_, err = s.Client.RemoveClientIDFromOpenIDConnectProvider(&iam.RemoveClientIDFromOpenIDConnectProviderInput{
						ClientID:                 &remove,
						OpenIDConnectProviderArn: &arn,
					})
					if err != nil {
						return microerror.Mask(err)
					}
					s.scope.Info(fmt.Sprintf("Removed client id %s to OIDCProvider for URL %s", remove, identityProviderURL))
				}

				if thumbprintsDiff.Changed() {
					s.scope.Info(fmt.Sprintf("Updating thumbprints on OIDCProvider for URL %s", identityProviderURL))
					_, err := s.Client.UpdateOpenIDConnectProviderThumbprint(&iam.UpdateOpenIDConnectProviderThumbprintInput{
						OpenIDConnectProviderArn: &arn,
						ThumbprintList:           thumbprints,
					})
					if err != nil {
						return microerror.Mask(err)
					}
					s.scope.Info(fmt.Sprintf("Updated thumbprints on OIDCProvider for URL %s", identityProviderURL))

				} else {
					s.scope.Info(fmt.Sprintf("OIDCProvider for URL %s already exists and is up to date", identityProviderURL))
				}
				break
			}
		}

		if found {
			continue
		}

		s.scope.Info(fmt.Sprintf("Creating OIDCProvider for URL %s", identityProviderURL))

		i := &iam.CreateOpenIDConnectProviderInput{
			Url:            aws.String(identityProviderURL),
			ThumbprintList: thumbprints,
			ClientIDList:   []*string{aws.String(clientID)},
		}

		// Add internal and customer tags.
		{
			for k, v := range s.internalTags() {
				tag := &iam.Tag{
					Key:   aws.String(k),
					Value: aws.String(v),
				}
				i.Tags = append(i.Tags, tag)
			}

			for k, v := range customerTags {
				tag := &iam.Tag{
					Key:   aws.String(k),
					Value: aws.String(v),
				}
				i.Tags = append(i.Tags, tag)
			}
		}

		_, err = s.Client.CreateOpenIDConnectProvider(i)
		if err != nil {
			return microerror.Mask(err)
		}
		s.scope.Info(fmt.Sprintf("Created OIDC provider for URL %s", identityProviderURL))
	}
	return nil
}

func (s *Service) internalTags() map[string]string {
	return map[string]string{
		key.S3TagOrganization: util.RemoveOrg(s.scope.ClusterNamespace()),
		key.S3TagCluster:      s.scope.ClusterName(),
		fmt.Sprintf(key.S3TagCloudProvider, s.scope.ClusterName()): "owned",
		key.S3TagInstallation: s.scope.Installation(),
	}
}

func (s *Service) findOIDCProviders() (map[string]*iam.GetOpenIDConnectProviderOutput, error) {
	s.scope.Info("Looking for existing OIDC providers")
	output, err := s.Client.ListOpenIDConnectProviders(&iam.ListOpenIDConnectProvidersInput{})
	if err != nil {
		return nil, microerror.Mask(err)
	}

	ret := make(map[string]*iam.GetOpenIDConnectProviderOutput, 0)

	for _, providerArn := range output.OpenIDConnectProviderList {
		p, err := s.Client.GetOpenIDConnectProvider(&iam.GetOpenIDConnectProviderInput{
			OpenIDConnectProviderArn: providerArn.Arn,
		})
		if err != nil {
			return nil, microerror.Mask(err)
		}

		// Check if tags match
		installationTagFound := false
		clusterTagFound := false
		for _, tag := range p.Tags {
			if *tag.Key == key.S3TagInstallation && *tag.Value == s.scope.Installation() {
				installationTagFound = true
			}
			if *tag.Key == key.S3TagCluster && *tag.Value == s.scope.ClusterName() {
				clusterTagFound = true
			}
		}

		if installationTagFound && clusterTagFound {
			ret[*providerArn.Arn] = p
		}
	}

	if len(ret) == 0 {
		s.scope.Info("Did not find any OIDC provider")
	} else {
		s.scope.Info(fmt.Sprintf("Found %d existing OIDC providers", len(ret)))
	}

	return ret, nil
}

func (s *Service) ListCustomerOIDCTags(release *semver.Version, cfDomain, accountID, bucketName, region string) (map[string]string, error) {
	var providerArn string
	if (key.IsV18Release(release) && !key.IsChina(region)) || (s.scope.MigrationNeeded() && !key.IsChina(region)) {
		providerArn = fmt.Sprintf("arn:%s:iam::%s:oidc-provider/%s", key.ARNPrefix(region), accountID, cfDomain)
	} else {
		providerArn = fmt.Sprintf("arn:%s:iam::%s:oidc-provider/s3.%s.%s/%s", key.ARNPrefix(region), accountID, region, key.AWSEndpoint(region), bucketName)
	}

	i := &iam.ListOpenIDConnectProviderTagsInput{
		OpenIDConnectProviderArn: aws.String(providerArn),
	}

	o, err := s.Client.ListOpenIDConnectProviderTags(i)
	if err != nil {
		return nil, err
	}

	ignoreKeyTags := []string{fmt.Sprintf(key.S3TagCloudProvider, s.scope.ClusterName()), key.S3TagCluster, key.S3TagInstallation, key.S3TagOrganization}
	oidcTags := make(map[string]string)
	for _, tag := range o.Tags {
		if !util.StringInSlice(aws.StringValue(tag.Key), ignoreKeyTags) {
			oidcTags[aws.StringValue(tag.Key)] = aws.StringValue(tag.Value)
		}
	}
	return oidcTags, nil
}

func (s *Service) DeleteOIDCProviders() error {
	providers, err := s.findOIDCProviders()
	if err != nil {
		return microerror.Mask(err)
	}

	for providerArn := range providers {
		i := &iam.DeleteOpenIDConnectProviderInput{
			OpenIDConnectProviderArn: aws.String(providerArn),
		}

		_, err := s.Client.DeleteOpenIDConnectProvider(i)
		if err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				case iam.ErrCodeNoSuchEntityException:
					s.scope.Info("OIDC provider no longer exists, skipping deletion")
					continue
				}
			}
			return err
		}
		s.scope.Info("Deleted OIDC provider")
	}

	return nil
}

func caThumbPrint(ep string) (string, error) {
	fp, err := sprint.GetFingerprint(ep, false)
	if err != nil {
		return "", err
	}
	return strings.Replace(fp.SHA1, ":", "", -1), nil
}
