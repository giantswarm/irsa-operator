package acm

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/aws/aws-sdk-go/service/acm/acmiface"
	"github.com/giantswarm/microerror"

	"github.com/giantswarm/irsa-operator/pkg/aws/services/route53"
	"github.com/giantswarm/irsa-operator/pkg/key"
	"github.com/giantswarm/irsa-operator/pkg/util"
)

func (s *Service) EnsureCertificate(domain string, customerTags map[string]string) (*string, error) {
	s.scope.Logger().Info(fmt.Sprintf("Ensuring ACM certificate for domain %q", domain))

	// Check if certificate exists
	certificateArn, err := s.findCertificateForDomain(domain)
	if err != nil {
		return nil, microerror.Mask(err)
	}

	if certificateArn != nil {
		s.scope.Logger().Info("ACM certificate already exists")

		return certificateArn, nil
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
	// add cluster tag if missing (this is case for vintage clusters)
	if _, ok := customerTags[key.S3TagCluster]; !ok {
		if customerTags == nil {
			customerTags = make(map[string]string)
		}
		customerTags[key.S3TagCluster] = s.scope.ClusterName()
	}

	for k, v := range customerTags {
		tag := &acm.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		}
		input.Tags = append(input.Tags, tag)
	}

	s.scope.Logger().Info("Creating ACM certificate")

	output, err := s.Client.RequestCertificate(input)
	if err != nil {
		return nil, microerror.Mask(err)
	}

	s.scope.Logger().Info("ACM certificate created successfully")
	return output.CertificateArn, nil
}

// IsCertificateIssued checks if an ACM certificate is issued.
func (s *Service) IsCertificateIssued(arn string) (bool, error) {
	s.scope.Logger().Info("Checking status of ACM certificate")

	output, err := s.Client.DescribeCertificate(&acm.DescribeCertificateInput{
		CertificateArn: aws.String(arn),
	})
	if err != nil {
		return false, err
	}

	return *output.Certificate.Status == acm.CertificateStatusIssued, nil
}

// IsValidated checks wheter an ACM certificate's ownership is already validated or not.
func (s *Service) IsValidated(arn string) (bool, error) {
	output, err := s.Client.DescribeCertificate(&acm.DescribeCertificateInput{
		CertificateArn: aws.String(arn),
	})
	if err != nil {
		return false, err
	}

	if len(output.Certificate.DomainValidationOptions) == 0 {
		return false, nil
	}

	renewalValidationPending := false
	if output.Certificate.RenewalSummary != nil && output.Certificate.RenewalSummary.RenewalStatus != nil {
		renewalValidationPending = *output.Certificate.RenewalSummary.RenewalStatus == acm.RenewalStatusPendingValidation
	}

	validated := *output.Certificate.DomainValidationOptions[0].ValidationStatus == acm.DomainStatusSuccess
	return validated && !renewalValidationPending, nil
}

// GetValidationCNAME returns a CNAME record that needs to be created in order for automated domain ownership validation to work.
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
	s.scope.Logger().Info("Ensuring ACM certificate is deleted")

	arn, err := s.findCertificateForDomain(domain)
	if err != nil {
		return microerror.Mask(err)
	}

	if arn != nil {
		s.scope.Logger().Info("Deleting ACM certificate")
		_, err = s.Client.DeleteCertificate(&acm.DeleteCertificateInput{CertificateArn: arn})
		if err != nil {
			return microerror.Mask(err)
		}

		s.scope.Logger().Info("Deleted ACM certificate")
		return nil
	}

	s.scope.Logger().Info("ACM certificate was not found")

	return nil
}

func (s *Service) findCertificateForDomain(domain string) (*string, error) {
	certs, err := getACMCertificates(s.Client)
	if err != nil {
		return nil, microerror.Mask(err)
	}
	for _, certificate := range certs {
		if *certificate.DomainName == domain {
			return certificate.CertificateArn, nil
		}
	}

	return nil, nil
}

func getACMCertificates(acmClient acmiface.ACMAPI) ([]*acm.CertificateSummary, error) {
	certs := []*acm.CertificateSummary{}
	listCertificatesOutput, err := acmClient.ListCertificates(&acm.ListCertificatesInput{
		MaxItems: aws.Int64(100),
	})
	if err != nil {
		return certs, microerror.Mask(err)
	}

	certs = append(certs, listCertificatesOutput.CertificateSummaryList...)

	// If the response contains `NexToken` we need to keep sending requests including the token to get all results.
	for listCertificatesOutput.NextToken != nil && *listCertificatesOutput.NextToken != "" {
		listCertificatesOutput, err = acmClient.ListCertificates(&acm.ListCertificatesInput{
			MaxItems:  aws.Int64(100),
			NextToken: listCertificatesOutput.NextToken,
		})
		if err != nil {
			return certs, microerror.Mask(err)
		}
		certs = append(certs, listCertificatesOutput.CertificateSummaryList...)
	}

	return certs, nil
}
