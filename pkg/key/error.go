package key

import "github.com/giantswarm/microerror"

var unexpectedApiEndpointError = &microerror.Error{
	Kind: "unexpectedApiEndpoint",
}

var missingApiEndpointError = &microerror.Error{
	Kind: "missingApiEndpoint",
}
