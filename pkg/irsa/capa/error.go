package capa

import "github.com/giantswarm/microerror"

var certificateNotIssuedError = &microerror.Error{
	Kind: "certificateNotIssuedError",
}

var clusterValuesConfigMapNotFound = &microerror.Error{
	Kind: "clusterValuesConfigMapNotFoundError",
}

var baseDomainNotFound = &microerror.Error{
	Kind: "baseDomainNotFoundError",
}
