package route53

import (
	"fmt"
	"strings"
	"time"

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
	s.scope.Logger().Info("Searching route53 hosted zone ID", "zoneName", zoneName)

	makeCacheKey := func(zoneName string, public bool) string {
		return fmt.Sprintf("route53/region=%q/arn=%q/zoneName=%q/public=%v/id", s.scope.Region(), s.scope.ARN(), zoneName, public)
	}

	if cachedValue, ok := s.scope.Cache().Get(makeCacheKey(zoneName, public)); ok {
		zoneId := cachedValue.(string)
		s.scope.Logger().Info("Using Route53 hosted zone ID from cache", "region", s.scope.Region(), "arn", s.scope.ARN(), "zoneId", zoneId, "zoneName", zoneName)
		return zoneId, nil
	}

	// Request up to the allowed maximum of 100 items. This way, we can get and cache the IDs of other
	// zones as well and thereby avoid making one request per zone name which can easily lead to AWS throttling
	// (Route53: 5 req/sec rate limit!). Mind that `DNSName` acts like an alphabetical start marker, not as equality
	// comparison - if that exact zone name does not exist, AWS may still return other zones!
	//
	// See https://docs.aws.amazon.com/Route53/latest/APIReference/API_ListHostedZonesByName.html.
	listResponse, err := s.Client.ListHostedZonesByName(&route53.ListHostedZonesByNameInput{
		DNSName:  aws.String(zoneName),
		MaxItems: aws.String("100"),
	})
	if err != nil {
		return "", microerror.Mask(err)
	}

	for _, zone := range listResponse.HostedZones {
		s.scope.Cache().Set(makeCacheKey(strings.TrimSuffix(*zone.Name, "."), !*zone.Config.PrivateZone), *zone.Id, 3*time.Minute)
	}

	// We return the first zone found that matches the basename and is public or not according to the parameter.
	wantedAWSZoneName := strings.TrimSuffix(zoneName, ".") + "."
	for _, zone := range listResponse.HostedZones {
		if *zone.Name == wantedAWSZoneName && public == !*zone.Config.PrivateZone {
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
