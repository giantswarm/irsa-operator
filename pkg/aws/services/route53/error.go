package route53

import "github.com/giantswarm/microerror"

var zoneNotFoundError = &microerror.Error{
	Kind: "zoneNotFoundError",
}
