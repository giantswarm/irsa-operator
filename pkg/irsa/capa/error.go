package capa

import (
	"reflect"

	"github.com/giantswarm/microerror"
)

var certificateNotIssuedError = &microerror.Error{
	Kind: "certificateNotIssuedError",
}

type CloudfrontDistributionNotDisabledError struct {
	error
}

func (e *CloudfrontDistributionNotDisabledError) Error() string {
	return "dns record type is not supported"
}

func (e *CloudfrontDistributionNotDisabledError) Is(target error) bool {
	return reflect.TypeOf(target) == reflect.TypeOf(e)
}
