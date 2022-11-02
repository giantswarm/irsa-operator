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
)

func (s *Service) EnsureOIDCProvider(identityProviderURL, clientID string) error {
	tp, err := caThumbPrint(identityProviderURL)
	if err != nil {
		return err
	}

	arn, existing, err := s.findOIDCProvider()
	if err != nil {
		return microerror.Mask(err)
	}

	if existing != nil {
		// Check if values are up to date.
		if *existing.Url != identityProviderURL ||
			len(existing.ThumbprintList) != 1 || *existing.ThumbprintList[0] != tp ||
			len(existing.ClientIDList) != 1 || *existing.ClientIDList[0] != clientID {

			if *existing.Url != identityProviderURL {
				fmt.Printf("url changed: was %q, is %q", *existing.Url, identityProviderURL)
			}
			if len(existing.ThumbprintList) != 1 || *existing.ThumbprintList[0] != tp {
				fmt.Println("tp changed")
			}
			if len(existing.ClientIDList) != 1 || *existing.ClientIDList[0] != clientID {
				fmt.Println("ClientID changed")
			}

			s.scope.Info("OIDCProvider needs to be replaced")
			s.scope.Info("Deleting old OIDCProvider")
			_, err = s.Client.DeleteOpenIDConnectProvider(&iam.DeleteOpenIDConnectProviderInput{OpenIDConnectProviderArn: aws.String(arn)})
			if err != nil {
				return microerror.Mask(err)
			}
			s.scope.Info("Deleted old OIDCProvider")
		} else {
			s.scope.Info("OIDCProvider already exists")
			return nil
		}
	}

	i := &iam.CreateOpenIDConnectProviderInput{
		Url:            aws.String(identityProviderURL),
		ThumbprintList: []*string{aws.String(tp)},
		ClientIDList:   []*string{aws.String(clientID)},
	}

	_, err = s.Client.CreateOpenIDConnectProvider(i)
	if err != nil {
		return microerror.Mask(err)
	}
	s.scope.Info("Created OIDC provider")

	return nil
}

func (s *Service) findOIDCProvider() (string, *iam.GetOpenIDConnectProviderOutput, error) {
	s.scope.Info("Looking for existing OIDC provider")
	output, err := s.Client.ListOpenIDConnectProviders(&iam.ListOpenIDConnectProvidersInput{})
	if err != nil {
		return "", nil, microerror.Mask(err)
	}

	for _, providerArn := range output.OpenIDConnectProviderList {
		p, err := s.Client.GetOpenIDConnectProvider(&iam.GetOpenIDConnectProviderInput{
			OpenIDConnectProviderArn: providerArn.Arn,
		})
		if err != nil {
			return "", nil, microerror.Mask(err)
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
			s.scope.Info("Found existing OIDC provider")
			return *providerArn.Arn, p, nil
		}
	}

	s.scope.Info("Did not find any OIDC provider")

	return "", nil, nil
}

func (s *Service) CreateOIDCTags(release *semver.Version, cfDomain, accountID, bucketName, region string, customerTags map[string]string) error {
	var providerArn string
	if (key.IsV18Release(release) && !key.IsChina(region)) || (s.scope.MigrationNeeded() && !key.IsChina(region)) {
		providerArn = fmt.Sprintf("arn:%s:iam::%s:oidc-provider/%s", key.ARNPrefix(region), accountID, cfDomain)
	} else {
		providerArn = fmt.Sprintf("arn:%s:iam::%s:oidc-provider/s3.%s.%s/%s", key.ARNPrefix(region), accountID, region, key.AWSEndpoint(region), bucketName)
	}
	i := &iam.TagOpenIDConnectProviderInput{
		OpenIDConnectProviderArn: aws.String(providerArn),
		Tags: []*iam.Tag{
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
	}

	for k, v := range customerTags {
		i.Tags = append(i.Tags, &iam.Tag{Key: aws.String(k), Value: aws.String(v)})
	}

	_, err := s.Client.TagOpenIDConnectProvider(i)
	if err != nil {
		return err
	}
	s.scope.Info("Created tags for OIDC provider")
	return nil
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

func (s *Service) RemoveOIDCTags(release *semver.Version, cfDomain, accountID, bucketName, region string, tagKeys []string) error {
	var providerArn string
	if (key.IsV18Release(release) && !key.IsChina(region)) || (s.scope.MigrationNeeded() && !key.IsChina(region)) {
		providerArn = fmt.Sprintf("arn:%s:iam::%s:oidc-provider/%s", key.ARNPrefix(region), accountID, cfDomain)
	} else {
		providerArn = fmt.Sprintf("arn:%s:iam::%s:oidc-provider/s3.%s.%s/%s", key.ARNPrefix(region), accountID, region, key.AWSEndpoint(region), bucketName)
	}
	i := &iam.UntagOpenIDConnectProviderInput{
		OpenIDConnectProviderArn: aws.String(providerArn),
		TagKeys:                  []*string{},
	}

	for _, t := range tagKeys {
		i.TagKeys = append(i.TagKeys, aws.String(t))
	}

	_, err := s.Client.UntagOpenIDConnectProvider(i)
	if err != nil {
		return err
	}
	s.scope.Info("Removed tags for OIDC provider")
	return nil
}

func (s *Service) DeleteOIDCProvider(release *semver.Version, cfDomain, accountID, bucketName, region string) error {
	var providerArn string
	if (key.IsV18Release(release) && !key.IsChina(region)) || (s.scope.MigrationNeeded() && !key.IsChina(region)) {
		providerArn = fmt.Sprintf("arn:%s:iam::%s:oidc-provider/%s", key.ARNPrefix(region), accountID, cfDomain)
	} else {
		providerArn = fmt.Sprintf("arn:%s:iam::%s:oidc-provider/s3.%s.%s/%s", key.ARNPrefix(region), accountID, region, key.AWSEndpoint(region), bucketName)
	}
	i := &iam.DeleteOpenIDConnectProviderInput{
		OpenIDConnectProviderArn: aws.String(providerArn),
	}

	_, err := s.Client.DeleteOpenIDConnectProvider(i)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case iam.ErrCodeNoSuchEntityException:
				s.scope.Info("OIDC provider no longer exists, skipping deletion")
				return nil
			}
		}
		return err
	}
	s.scope.Info("Deleted OIDC provider")

	return nil
}

func caThumbPrint(ep string) (string, error) {
	fp, err := sprint.GetFingerprint(ep, false)
	if err != nil {
		return "", err
	}
	return strings.Replace(fp.SHA1, ":", "", -1), nil
}
