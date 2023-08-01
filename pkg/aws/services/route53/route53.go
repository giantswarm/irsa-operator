package route53

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/giantswarm/microerror"
)

type CNAME struct {
	Name  string
	Value string
}

func (s *Service) FindPublicHostedZone(basename string) (string, error) {
	return s.findHostedZone(basename, true)
}

func (s *Service) FindPrivateHostedZone(basename string) (string, error) {
	return s.findHostedZone(basename, false)
}

func (s *Service) findHostedZone(zoneName string, public bool) (string, error) {
	s.scope.Logger().Info("Searching route53 hosted zone ID")

	listResponse, err := s.Client.ListHostedZonesByName(&route53.ListHostedZonesByNameInput{
		DNSName: aws.String(zoneName),
	})
	if err != nil {
		return "", microerror.Mask(err)
	}

	// We return the first zone found that matches the basename and is public or not according to the parameter.
	for _, zone := range listResponse.HostedZones {
		if *zone.Name == fmt.Sprintf("%s.", strings.TrimSuffix(zoneName, ".")) && public == !*zone.Config.PrivateZone {
			return *zone.Id, nil
		}
	}

	return "", microerror.Mask(zoneNotFoundError)
}

func (s *Service) EnsureDNSRecord(hostedZoneID string, cname CNAME) error {
	s.scope.Logger().Info(fmt.Sprintf("Ensuring CNAME record %q in zone %q", cname.Name, hostedZoneID))

	input := &route53.ChangeResourceRecordSetsInput{
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				{
					Action: aws.String(route53.ChangeActionUpsert),
					ResourceRecordSet: &route53.ResourceRecordSet{
						Name: aws.String(cname.Name),
						ResourceRecords: []*route53.ResourceRecord{
							{
								Value: aws.String(cname.Value),
							},
						},
						TTL:  aws.Int64(600),
						Type: aws.String(route53.RRTypeCname),
					},
				},
			},
		},
		HostedZoneId: aws.String(hostedZoneID),
	}

	_, err := s.Client.ChangeResourceRecordSets(input)
	if err != nil {
		return microerror.Mask(err)
	}

	s.scope.Logger().Info(fmt.Sprintf("Ensured CNAME record %q in zone %q", cname.Name, hostedZoneID))

	return nil
}
