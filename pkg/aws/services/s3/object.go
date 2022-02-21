package s3

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
)

func (s *Service) Upload(ctx context.Context, bucketName string, objects []string) {
	for _, obj := range objects {
		file, err := os.Open(obj)
		if err != nil {
			fmt.Println("Unable to open file " + obj)
			return
		}

		i := s3.PutObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(obj),
			Body:   file,
		}
		_, err = s.Client.PutObject(&i)
		if err != nil {
			fmt.Println("Unable to upload file " + obj)
			return
		}
	}
}
