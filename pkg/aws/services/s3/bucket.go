package s3

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/giantswarm/irsa-operator/pkg/key"
	"github.com/giantswarm/irsa-operator/pkg/util"
)

// S3BucketEncryptionAlgorithm is used to determine which algorithm use S3 to encrypt buckets.
const S3BucketEncryptionAlgorithm = "AES256"

func (s *Service) CreateBucket(bucketName string) error {
	i := &s3.CreateBucketInput{
		ACL:             aws.String("private"),
		Bucket:          aws.String(bucketName),
		ObjectOwnership: aws.String(s3.ObjectOwnershipObjectWriter),
	}
	_, err := s.Client.CreateBucket(i)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case s3.ErrCodeBucketAlreadyOwnedByYou:
				s.scope.Logger().Info("Bucket already exists", "bucket", bucketName)
				return nil
			case s3.ErrCodeBucketAlreadyExists:
				s.scope.Logger().Info("Bucket already exists", "bucket", bucketName)
				return nil
			}
		}
		return err
	}
	s.scope.Logger().Info("Created bucket", "bucket", bucketName)

	return nil
}

func (s *Service) CreateTags(bucketName string, customerTags map[string]string) error {
	i := &s3.PutBucketTaggingInput{
		Bucket: aws.String(bucketName),
		Tagging: &s3.Tagging{
			TagSet: []*s3.Tag{
				{
					Key:   aws.String(key.S3TagOrganization),
					Value: aws.String(util.RemoveOrg(s.scope.ClusterNamespace())),
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
	}
	// add cluster tag if missing (this is case for vintage clusters)
	if _, ok := customerTags[key.S3TagCluster]; !ok {
		if customerTags == nil {
			customerTags = make(map[string]string)
		}
		customerTags[key.S3TagCluster] = s.scope.ClusterName()
	}

	for k, v := range customerTags {
		i.Tagging.TagSet = append(i.Tagging.TagSet, &s3.Tag{Key: aws.String(k), Value: aws.String(v)})
	}

	i.Tagging.TagSet = util.FilterUniqueTags(i.Tagging.TagSet)

	_, err := s.Client.PutBucketTagging(i)
	if err != nil {
		return err
	}

	s.scope.Logger().Info("Created tags for S3 bucket", "bucket", bucketName)
	return nil
}

func (s *Service) EncryptBucket(bucketName string) error {
	i := &s3.PutBucketEncryptionInput{
		Bucket: aws.String(bucketName),
		ServerSideEncryptionConfiguration: &s3.ServerSideEncryptionConfiguration{
			Rules: []*s3.ServerSideEncryptionRule{
				{
					ApplyServerSideEncryptionByDefault: &s3.ServerSideEncryptionByDefault{
						SSEAlgorithm: aws.String(S3BucketEncryptionAlgorithm),
					},
				},
			},
		},
	}

	_, err := s.Client.PutBucketEncryption(i)
	if err != nil {
		return err
	}

	s.scope.Logger().Info("Encrypted S3 bucket", "bucket", bucketName)
	return nil
}

func (s *Service) DeleteBucket(bucketName string) error {
	i := &s3.DeleteBucketInput{
		Bucket: aws.String(bucketName),
	}
	_, err := s.Client.DeleteBucket(i)

	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case s3.ErrCodeNoSuchBucket:
				s.scope.Logger().Info("Bucket does not exist, skipping bucket deletion", "bucket", bucketName)
				return nil
			}
		}
		return err
	}
	s.scope.Logger().Info("Deleted bucket", "bucket", bucketName)
	return nil
}

func (s *Service) IsBucketReady(bucketName string) error {
	i := &s3.HeadBucketInput{
		Bucket: aws.String(bucketName),
	}

	_, err := s.Client.HeadBucket(i)
	if err != nil {
		return err
	}
	s.scope.Logger().Info("S3 bucket already exists, skipping creation", "bucket", bucketName)
	return nil
}

func (s *Service) UpdatePolicy(bucketName, oaiId string) error {
	var cloudfrontPolicy = `{
	"Version": "2012-10-17",
	"Id": "PolicyForCloudFrontPrivateContent",
	"Statement": [
		{
			"Effect": "Allow",
			"Principal": {
				"AWS": "arn:{{.ARNPrefix}}:iam::cloudfront:user/CloudFront Origin Access Identity {{.CloudFrontOriginAccessIdentityId}}"
			},
			"Action": "s3:GetObject",
			"Resource": "arn:{{.ARNPrefix}}:s3:::{{.BucketName}}/*"
		},
		{
			"Sid": "ForceSSLOnlyAccess",
			"Effect": "Deny",
			"Principal": "*",
			"Action": "s3:*",
			"Resource": "arn:{{.ARNPrefix}}:s3:::{{.BucketName}}/*",
			"Condition": {
			  "Bool": {
				"aws:SecureTransport": "false"
			  }
			}
		}
	]
}`

	t, err := template.New("").Parse(cloudfrontPolicy)
	if err != nil {
		return err
	}
	values := struct {
		ARNPrefix                        string
		BucketName                       string
		CloudFrontOriginAccessIdentityId string
	}{
		key.ARNPrefix(s.scope.Region()),
		bucketName,
		oaiId,
	}

	var buf bytes.Buffer
	err = t.Execute(&buf, values)
	if err != nil {
		return err
	}
	_, err = s.Client.PutBucketPolicy(&s3.PutBucketPolicyInput{
		Bucket: aws.String(bucketName),
		Policy: aws.String(buf.String()),
	})
	if err != nil {
		return err
	}

	s.scope.Logger().Info("Restricted access to allow Cloudfront reaching S3 bucket", "bucket", bucketName)
	return nil
}

func (s *Service) BlockPublicAccess(bucketName string) error {
	i := &s3.PutPublicAccessBlockInput{
		Bucket: aws.String(bucketName),
		PublicAccessBlockConfiguration: &s3.PublicAccessBlockConfiguration{
			BlockPublicAcls:       aws.Bool(true),
			BlockPublicPolicy:     aws.Bool(true),
			IgnorePublicAcls:      aws.Bool(true),
			RestrictPublicBuckets: aws.Bool(true),
		},
	}
	_, err := s.Client.PutPublicAccessBlock(i)
	if err != nil {
		return err
	}
	s.scope.Logger().Info("Blocked public access for S3 bucket", "bucket", bucketName)
	return nil

}

func (s *Service) AllowPublicAccess(bucketName string) error {
	i := &s3.PutPublicAccessBlockInput{
		Bucket: aws.String(bucketName),
		PublicAccessBlockConfiguration: &s3.PublicAccessBlockConfiguration{
			BlockPublicAcls:       aws.Bool(false),
			BlockPublicPolicy:     aws.Bool(false),
			IgnorePublicAcls:      aws.Bool(false),
			RestrictPublicBuckets: aws.Bool(false),
		},
	}
	_, err := s.Client.PutPublicAccessBlock(i)
	if err != nil {
		return err
	}
	s.scope.Logger().Info("Allowed public access for S3 bucket", "bucket", bucketName)
	return nil

}
