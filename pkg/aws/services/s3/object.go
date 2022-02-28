package s3

import (
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
)

var objects = []string{"discovery.json", "keys.json"}

func (s *Service) UploadFiles(bucketName string) error {
	for _, obj := range objects {
		file, err := os.Open(fmt.Sprintf("/tmp/%s/%s", bucketName, obj))
		if err != nil {
			return err
		}
		defer file.Close()

		if obj == "discovery.json" {
			obj = "/.well-known/openid-configuration"
		}

		i := s3.PutObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(obj),
			ACL:    aws.String("public-read"),
			Body:   file,
		}
		_, err = s.Client.PutObject(&i)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) DeleteFiles(bucketName string) error {

	deleteObjects := []*s3.ObjectIdentifier{}
	for _, obj := range objects {
		if obj == "discovery.json" {
			obj = ".well-known/openid-configuration"
		}
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
		return err
	}
	return nil
}
