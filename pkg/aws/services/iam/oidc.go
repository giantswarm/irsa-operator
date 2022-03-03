package iam

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/nhalstead/sprint"
)

const clientID = "sts.amazonaws.com"

func (s *Service) CreateOIDCProvider(bucketName, region string) error {

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
				return nil
			}
		}
		return err
	}

	return nil
}

//Example OIDC ARN arn:aws:iam::ACCOUNT_ID:oidc-provider/s3-S3_REGION.amazonaws.com/BUCKET_NAME
func (s *Service) DeleteOIDCProvider(accountID, bucketName, region string) error {

	providerArn := fmt.Sprintf("arn:aws:iam::%s:oidc-provider/s3-%s.amazonaws.com/%s", accountID, region, bucketName)

	i := &iam.DeleteOpenIDConnectProviderInput{
		OpenIDConnectProviderArn: aws.String(providerArn),
	}

	_, err := s.Client.DeleteOpenIDConnectProvider(i)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case iam.ErrCodeNoSuchEntityException:
				return nil
			}
		}
		return err
	}

	return nil
}

func caThumbPrint(ep string) (string, error) {
	fp, _ := sprint.GetFingerprint(ep, false)
	return fp.SHA1, nil
}

func removeColon(value string) string {
	return strings.Replace(value, ":", "", -1)
}
