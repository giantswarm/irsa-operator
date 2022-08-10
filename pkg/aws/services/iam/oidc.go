package iam

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/blang/semver"
	"github.com/nhalstead/sprint"

	"github.com/giantswarm/irsa-operator/pkg/key"
	"github.com/giantswarm/irsa-operator/pkg/util"
)

func (s *Service) CreateOIDCProvider(release *semver.Version, domain, bucketName, region string) error {
	s3Endpoint := fmt.Sprintf("s3.%s.%s", region, key.AWSEndpoint(region))

	var identityProviderURL string
	if key.IsV18Release(release) && !key.IsChina(region) {
		identityProviderURL = fmt.Sprintf("https://%s", domain)
	} else {
		identityProviderURL = fmt.Sprintf("https://%s/%s", s3Endpoint, bucketName)
	}

	tp, err := caThumbPrint(s3Endpoint)
	if err != nil {
		return err
	}

	i := &iam.CreateOpenIDConnectProviderInput{
		Url:            aws.String(identityProviderURL),
		ThumbprintList: []*string{aws.String(removeColon(tp))},
		ClientIDList:   []*string{aws.String(fmt.Sprintf("sts.%s", key.AWSEndpoint(region)))},
	}

	_, err = s.Client.CreateOpenIDConnectProvider(i)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case iam.ErrCodeEntityAlreadyExistsException:
				s.scope.Info("OIDC provider already exists, skipping creation")
				return nil
			}
		}
		return err
	}
	s.scope.Info("Created OIDC provider")

	return nil
}

func (s *Service) CreateOIDCTags(accountID, bucketName, region string, customerTags map[string]string) error {
	providerArn := fmt.Sprintf("arn:%s:iam::%s:oidc-provider/s3.%s.%s/%s", key.ARNPrefix(region), accountID, region, key.AWSEndpoint(region), bucketName)
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

func (s *Service) ListCustomerOIDCTags(accountID, bucketName, region string) (map[string]string, error) {
	s.scope.Info("Listing OIDC tags")

	providerArn := fmt.Sprintf("arn:%s:iam::%s:oidc-provider/s3.%s.%s/%s", key.ARNPrefix(region), accountID, region, key.AWSEndpoint(region), bucketName)
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

func (s *Service) RemoveOIDCTags(accountID, bucketName, region string, tagKeys []string) error {
	providerArn := fmt.Sprintf("arn:%s:iam::%s:oidc-provider/s3.%s.%s/%s", key.ARNPrefix(region), accountID, region, key.AWSEndpoint(region), bucketName)
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
	if key.IsV18Release(release) && !key.IsChina(region) {
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
	fp, _ := sprint.GetFingerprint(ep, false)
	return fp.SHA1, nil
}

func removeColon(value string) string {
	return strings.Replace(value, ":", "", -1)
}
