package iam

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/nhalstead/sprint"

	"github.com/giantswarm/irsa-operator/pkg/key"
	"github.com/giantswarm/irsa-operator/pkg/util"
)

const clientID = "sts.amazonaws.com"

func (s *Service) CreateOIDCProvider(bucketName, region string) error {
	s.scope.Info("Creating OIDC provider")

	s3Endpoint := fmt.Sprintf("s3-%s.amazonaws.com", region)
	identityProviderURL := fmt.Sprintf("https://%s/%s", s3Endpoint, bucketName)

	tp, err := caThumbPrint(s3Endpoint)
	if err != nil {
		return err
	}

	i := &iam.CreateOpenIDConnectProviderInput{
		Url:            aws.String(identityProviderURL),
		ThumbprintList: []*string{aws.String(removeColon(tp))},
		ClientIDList:   []*string{aws.String(clientID)},
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
	s.scope.Info("Creating tags on OIDC provider")

	providerArn := fmt.Sprintf("arn:aws:iam::%s:oidc-provider/s3-%s.amazonaws.com/%s", accountID, region, bucketName)
	i := &iam.TagOpenIDConnectProviderInput{
		OpenIDConnectProviderArn: aws.String(providerArn),
		Tags: []*iam.Tag{
			{
				Key:   aws.String(key.S3TagOrganization),
				Value: aws.String(s.scope.ClusterNamespace()),
			},
			{
				Key:   aws.String(key.S3TagCluster),
				Value: aws.String(s.scope.ClusterName()),
			},
			{
				Key:   aws.String(fmt.Sprintf(key.S3TagCloudProvider, util.RemoveOrg(s.scope.ClusterNamespace()))),
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
	return nil
}

// Example OIDC ARN arn:aws:iam::ACCOUNT_ID:oidc-provider/s3-S3_REGION.amazonaws.com/BUCKET_NAME

func (s *Service) DeleteOIDCProvider(accountID, bucketName, region string) error {
	s.scope.Info("Deleting OIDC provider")

	providerArn := fmt.Sprintf("arn:aws:iam::%s:oidc-provider/s3-%s.amazonaws.com/%s", accountID, region, bucketName)

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
