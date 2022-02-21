package s3

import (
	"context"
	"errors"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go/service/s3"
)

func (s *Service) Create(ctx context.Context, bucketName string) error {
	i := &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	}

	_, err := s.Client.CreateBucket(i)
	if err != nil {
		var bae *types.BucketAlreadyExists
		var bao *types.BucketAlreadyOwnedByYou
		if errors.As(err, &bae) {
			log.Printf("Bucket %s already exists, skip creation", bucketName)
			return nil
		} else if errors.As(err, &bao) {
			// Fall through
		} else {
			return err
		}
	}

	return nil
}
