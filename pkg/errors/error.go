package errors

import (
	"errors"
	"reflect"

	"github.com/giantswarm/irsa-operator/pkg/aws/services/cloudfront"
)

func IsEmptyCloudfrontDistribution(distribution *cloudfront.Distribution) error {
	if reflect.ValueOf(*distribution).IsZero() {
		return errors.New("empty cloudfront distribution")
	}
	return nil
}
