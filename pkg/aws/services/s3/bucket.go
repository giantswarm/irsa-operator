package s3

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
)

func (s *Service) CreateBucket(bucketName string) error {
	i := &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	}
	s.scope.Info("Creating bucket", "bucket", bucketName)

	_, err := s.Client.CreateBucket(i)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case s3.ErrCodeBucketAlreadyOwnedByYou:
				s.scope.Info("Bucket already exists", "bucket", bucketName)
				return nil
			case s3.ErrCodeBucketAlreadyExists:
				s.scope.Info("Bucket already exists", "bucket", bucketName)
				return nil
			}
		}
		return err
	}
	s.scope.Info("Created bucket", "bucket", bucketName)

	return nil
}

func (s *Service) DeleteBucket(bucketName string) error {
	i := &s3.DeleteBucketInput{
		Bucket: aws.String(bucketName),
	}
	s.scope.Info("Deleting bucket", "bucket", bucketName)

	_, err := s.Client.DeleteBucket(i)

	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case s3.ErrCodeNoSuchBucket:
				s.scope.Info("Bucket do not exist, continue with deletion", "bucket", bucketName)
				return nil
			}
		}
		return err
	}
	s.scope.Info("Deleted bucket", "bucket", bucketName)

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
	return nil
}
