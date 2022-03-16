package s3

import (
	"crypto/rsa"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/giantswarm/microerror"

	oidc2 "github.com/giantswarm/irsa-operator/pkg/oidc"
)

var objects = []string{".well-known/openid-configuration", "keys.json"}

func (s *Service) UploadFiles(bucketName string, key *rsa.PrivateKey) error {
	s.scope.Info(fmt.Sprintf("Uploading files to bucket"), "bucket", bucketName)

	// discovery file '/.well-known/openid-configuration'
	{
		fileName := "/.well-known/openid-configuration"
		i0 := &s3.HeadObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(fileName),
		}

		_, err := s.Client.HeadObject(i0)
		if err != nil {
			discoveryFile, err := oidc2.GenerateDiscoveryFile(bucketName, s.scope.Region())
			if err != nil {
				return microerror.Mask(err)
			}

			i := s3.PutObjectInput{
				Bucket: aws.String(bucketName),
				Key:    aws.String(fileName),
				ACL:    aws.String("public-read"),
				Body:   discoveryFile,
			}
			_, err = s.Client.PutObject(&i)
			if err != nil {
				return microerror.Mask(err)
			}
			s.scope.Info(fmt.Sprintf("Uploaded '%s'", fileName), "bucket", bucketName)

		} else {
			s.scope.Info(fmt.Sprintf("File '%s', already exist, skipping the update", fileName), "bucket", bucketName)
		}
	}
	// keys file 'keys.json'
	{
		fileName := "keys.json"
		i0 := &s3.HeadObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(fileName),
		}
		_, err := s.Client.HeadObject(i0)
		if err != nil {
			keysFile, err := oidc2.GenerateKeysFile(key)
			if err != nil {
				return microerror.Mask(err)
			}

			i := s3.PutObjectInput{
				Bucket: aws.String(bucketName),
				Key:    aws.String("keys.json"),
				ACL:    aws.String("public-read"),
				Body:   keysFile,
			}
			_, err = s.Client.PutObject(&i)
			if err != nil {
				return microerror.Mask(err)
			}
			s.scope.Info(fmt.Sprintf("Uploaded '%s'", fileName), "bucket", bucketName)

		} else {
			s.scope.Info(fmt.Sprintf("File '%s', already exist, skipping the update", fileName), "bucket", bucketName)

		}
	}
	s.scope.Info(fmt.Sprintf("Uploaded files to bucket"), "bucket", bucketName)

	return nil
}

func (s *Service) DeleteFiles(bucketName string) error {
	s.scope.Info(fmt.Sprintf("Deleting %d files from bucket", len(objects)), "bucket", bucketName)

	var deleteObjects []*s3.ObjectIdentifier
	for _, obj := range objects {
		deleteObjects = append(deleteObjects, &s3.ObjectIdentifier{
			Key: aws.String(obj),
		})
	}

	i := s3.DeleteObjectsInput{
		Bucket: aws.String(bucketName),
		Delete: &s3.Delete{
			Objects: deleteObjects,
		},
	}

	_, err := s.Client.DeleteObjects(&i)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case s3.ErrCodeNoSuchBucket:
				s.scope.Info("Bucket do not exist, continue with deletion", "bucket", bucketName)
				return nil
			case s3.ErrCodeNoSuchKey:
				s.scope.Info("Files do not exist, continue with deletion", "bucket", bucketName)
			}
		}
		return err
	}
	s.scope.Info(fmt.Sprintf("Deleted %d files from bucket", len(objects)), "bucket", bucketName)

	return nil
}
