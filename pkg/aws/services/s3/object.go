package s3

import (
	"bytes" //#nosec
	"crypto/rsa"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/blang/semver"
	"github.com/giantswarm/microerror"
	"github.com/peak/s3hash"

	"github.com/giantswarm/irsa-operator/pkg/key"
	oidc2 "github.com/giantswarm/irsa-operator/pkg/oidc"
)

var objects = []string{".well-known/openid-configuration", "keys.json"}

type FileObject struct {
	FileName    string
	Content     *bytes.Reader
	ContentType string
}

func (s *Service) UploadFiles(release *semver.Version, domain, bucketName string, privateKey *rsa.PrivateKey) error {
	discoveryFile, err := oidc2.GenerateDiscoveryFile(release, domain, bucketName, s.scope.Region(), s.scope.MigrationNeeded())
	if err != nil {
		return microerror.Mask(err)
	}

	keysFile, err := oidc2.GenerateKeysFile(privateKey)
	if err != nil {
		return microerror.Mask(err)
	}

	files := []FileObject{
		{
			FileName:    objects[0],
			Content:     discoveryFile,
			ContentType: "application/json",
		},
		{
			FileName:    objects[1],
			Content:     keysFile,
			ContentType: "application/json",
		},
	}

	s.scope.Info("Uploading files to bucket", "bucket", bucketName)
	for _, i := range files {
		content := *i.Content
		fileName := i.FileName
		i0 := &s3.HeadObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(fileName),
		}

		eTagCalc, err := s3hash.Calculate(i.Content, int64(i.Content.Len()))
		if err != nil {
			return microerror.Mask(err)
		}

		var update bool
		ho, err := s.Client.HeadObject(i0)
		if ho.ETag != nil {
			if strings.Replace(*ho.ETag, "\"", "", -1) != eTagCalc {
				s.scope.Info(fmt.Sprintf("Hashdiff of object '%s' detected, reuploading", fileName), "bucket", bucketName)
				update = true
			}

		}

		if err != nil || update {
			input := s3.PutObjectInput{
				Bucket:        aws.String(bucketName),
				Key:           aws.String(fileName),
				ACL:           aws.String("public-read"),
				ContentType:   aws.String(i.ContentType),
				ContentLength: aws.Int64(int64(content.Len())),

				Body: &content,
			}

			if (key.IsV18Release(release) && !key.IsChina(s.scope.Region())) || (s.scope.MigrationNeeded() && !key.IsChina(s.scope.Region())) {
				input.ACL = aws.String("private")
			}
			_, err = s.Client.PutObject(&input)
			if err != nil {
				s.scope.Info(fmt.Sprintf("failed to upload file: %v", err.Error()))
				return microerror.Mask(err)
			}
			s.scope.Info(fmt.Sprintf("Uploaded '%s'", fileName), "bucket", bucketName)

		} else {
			s.scope.Info(fmt.Sprintf("File '%s', already exist, skipping the update", fileName), "bucket", bucketName)
		}
	}

	return nil
}

func (s *Service) DeleteFiles(bucketName string) error {
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
				s.scope.Info("Bucket does not exist, skipping files deletion", "bucket", bucketName)
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
