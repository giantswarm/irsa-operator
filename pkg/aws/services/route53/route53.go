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
		return fmt.Sprintf("route53/arn=%q/zoneName=%q/public=%v/id", s.scope.ARN(), zoneName, public)
	}

	if cachedValue, ok := s.scope.Cache().Get(makeCacheKey(zoneName, public)); ok {
		zoneId := cachedValue.(string)
		s.scope.Logger().Info("Using Route53 hosted zone ID from cache", "arn", s.scope.ARN(), "zoneId", zoneId, "zoneName", zoneName)
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
		s.scope.Cache().Set(
			makeCacheKey(strings.TrimSuffix(*zone.Name, "."), !*zone.Config.PrivateZone),
			*zone.Id,
			// We requeue every few minutes to update OIDC certificate thumbprints (see controller code), and there's no
			// reason to think that a DNS zone ID was changed/deleted for the purposes of irsa-operator. So cache results
			// long enough to last 2 reconciliations (= cache longer than controller's requeue interval).
			7*time.Minute)
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
	logger := s.scope.Logger().WithValues("zoneId", hostedZoneID, "name", cname.Name, "value", cname.Value)

	logger.Info("Ensuring CNAME record")

	cacheKey := fmt.Sprintf("route53/arn=%q/zoneId=%q/cname-name=%q/cname-value", s.scope.ARN(), hostedZoneID, cname.Name)

	if cachedValue, ok := s.scope.Cache().Get(cacheKey); ok {
		cachedCNAMEValue := cachedValue.(string)

		if cachedCNAMEValue == cname.Value {
			// Avoid making excess Route53 requests that lead to rate limiting if we recently
			// upserted this exact CNAME record
			logger.Info("CNAME record was recently ensured, skipping upsert")
			return nil
		}
	}

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

	// No other operator touches the `irsa.<basedomain>` DNS record, so we can remember for a long period
	// that the DNS record was already upserted.
	s.scope.Cache().Set(cacheKey, cname.Value, 10*time.Minute)

	logger.Info("Ensured CNAME record")

	return nil
}
