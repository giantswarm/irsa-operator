package cloudfront

import (
	"reflect"

	"github.com/giantswarm/microerror"
)

var invalidOriginAccessIdentity = &microerror.Error{
	Kind: "invalidOriginAccessIdentity",
}

type DistributionNotDisabledError struct {
	error
}

func (e *DistributionNotDisabledError) Error() string {
	return "dns record type is not supported"
}

func (e *DistributionNotDisabledError) Is(target error) bool {
	return reflect.TypeOf(target) == reflect.TypeOf(e)
}
