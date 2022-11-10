package scope

import (
	"github.com/giantswarm/irsa-operator/pkg/aws"
)

// ACMScope is a scope for use with the ACM reconciling service in cluster
type ACMScope interface {
	aws.ClusterScoper
}

// CloudfrontScope is a scope for use with the Cloudfront reconciling service in cluster
type CloudfrontScope interface {
	aws.ClusterScoper
}

// IAMScope is a scope for use with the IAM reconciling service in cluster
type IAMScope interface {
	aws.ClusterScoper
}

// Route53Scope is a scope for use with the route53 reconciling service in cluster
type Route53Scope interface {
	aws.ClusterScoper
}

// S3Scope is a scope for use with the S3 reconciling service in cluster
type S3Scope interface {
	aws.ClusterScoper
}
