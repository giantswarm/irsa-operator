package scope

import (
	"github.com/giantswarm/irsa-operator/pkg/aws"
)

// CloudfrontScope is a scope for use with the Cloudfront reconciling service in cluster
type CloudfrontScope interface {
	aws.ClusterScoper
}

// IAMScope is a scope for use with the IAM reconciling service in cluster
type IAMScope interface {
	aws.ClusterScoper
}

// S3Scope is a scope for use with the S3 reconciling service in cluster
type S3Scope interface {
	aws.ClusterScoper
}
