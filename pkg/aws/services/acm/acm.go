package acm

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/giantswarm/microerror"

	"github.com/giantswarm/irsa-operator/pkg/aws/services/route53"
	"github.com/giantswarm/irsa-operator/pkg/key"
	"github.com/giantswarm/irsa-operator/pkg/util"
)

func (s *Service) EnsureCertificate(domain string, customerTags map[string]string) (*string, error) {
	s.scope.Info(fmt.Sprintf("Ensuring ACM certificate for domain %q", domain))

	// Check if certificate exists
	existing, err := s.findCertificateForDomain(domain)
	if err != nil {
		if err != nil {
			return nil, microerror.Mask(err)
		}
	}

	if existing != nil {
		s.scope.Info("ACM certificate already exists")

		return existing, nil
	}

	input := &acm.RequestCertificateInput{
		DomainName: aws.String(domain),
		Options:    &acm.CertificateOptions{},
		Tags: []*acm.Tag{
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
		ValidationMethod: aws.String(acm.ValidationMethodDns),
	}

	for k, v := range customerTags {
		tag := &acm.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		}
		input.Tags = append(input.Tags, tag)
	}

	s.scope.Info("Creating ACM certificate")

	output, err := s.Client.RequestCertificate(input)
	if err != nil {
		return nil, microerror.Mask(err)
	}

	s.scope.Info("ACM certificate created successfully")
	return output.CertificateArn, nil
}

func (s *Service) IsCertificateIssued(arn string) (bool, error) {
	s.scope.Info("Checking status of ACM certificate")

	output, err := s.Client.DescribeCertificate(&acm.DescribeCertificateInput{
		CertificateArn: aws.String(arn),
	})
	if err != nil {
		return false, err
	}

	return *output.Certificate.Status == acm.CertificateStatusIssued, nil
}

func (s *Service) IsValidated(arn string) (bool, error) {
	output, err := s.Client.DescribeCertificate(&acm.DescribeCertificateInput{
		CertificateArn: aws.String(arn),
	})
	if err != nil {
		return false, err
	}

	return *output.Certificate.DomainValidationOptions[0].ValidationStatus == acm.DomainStatusSuccess, nil
}

func (s *Service) GetValidationCNAME(arn string) (*route53.CNAME, error) {
	output, err := s.Client.DescribeCertificate(&acm.DescribeCertificateInput{
		CertificateArn: aws.String(arn),
	})
	if err != nil {
		return nil, err
	}

	// If certificate is just created, validation data might be missing.
	if len(output.Certificate.DomainValidationOptions) == 0 ||
		output.Certificate.DomainValidationOptions[0].ResourceRecord == nil ||
		output.Certificate.DomainValidationOptions[0].ResourceRecord.Name == nil ||
		output.Certificate.DomainValidationOptions[0].ResourceRecord.Value == nil {
		return nil, microerror.Mask(domainValidationDnsRecordNotFound)
	}

	return &route53.CNAME{
		Name:  *output.Certificate.DomainValidationOptions[0].ResourceRecord.Name,
		Value: *output.Certificate.DomainValidationOptions[0].ResourceRecord.Value,
	}, nil
}

func (s *Service) DeleteCertificate(domain string) error {
	s.scope.Info("Ensuring ACM certificate is deleted")

	arn, err := s.findCertificateForDomain(domain)
	if err != nil {
		return microerror.Mask(err)
	}

	if arn != nil {
		s.scope.Info("Deleting ACM certificate")
		_, err = s.Client.DeleteCertificate(&acm.DeleteCertificateInput{CertificateArn: arn})
		if err != nil {
			return microerror.Mask(err)
		}

		s.scope.Info("Deleted ACM certificate")
		return nil
	}

	s.scope.Info("ACM certificate was not found")

	return nil
}

func (s *Service) findCertificateForDomain(domain string) (*string, error) {
	var existing *acm.ListCertificatesOutput
	var err error

	// NextToken is the way AWS API performs pagination over results.
	// If NextToken is not nil, there is another page of results to be requested.
	// If existing is nil, means we have to request the very first page of results.
	for existing == nil || existing.NextToken != nil {
		var nextToken *string
		if existing != nil {
			nextToken = existing.NextToken
		}
		existing, err = s.Client.ListCertificates(&acm.ListCertificatesInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, microerror.Mask(err)
		}

		if len(existing.CertificateSummaryList) == 0 {
			return nil, nil
		}

		for _, c := range existing.CertificateSummaryList {
			if *c.DomainName == domain {
				return c.CertificateArn, nil
			}
		}
	}

	return nil, nil
}
