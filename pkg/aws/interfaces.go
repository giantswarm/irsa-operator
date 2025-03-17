package aws

import (
	awsclient "github.com/aws/aws-sdk-go/aws/client"
	"github.com/go-logr/logr"
	gocache "github.com/patrickmn/go-cache"
	"k8s.io/apimachinery/pkg/runtime"
)

// Session represents an AWS session
type Session interface {
	Session() awsclient.ConfigProvider
}

// ClusterScoper is the interface for a workload cluster scope
type ClusterScoper interface {
	Session

	// Logger retrieves the logger
	Logger() logr.Logger

	// ARN returns the workload cluster assumed role to operate.
	ARN() string
	// ManagementClusterIAMRoleArn returns the IAM Role to assume when changing resources in the MC account.
	ManagementClusterIAMRoleArn() string
	// BucketName returns the AWS infrastructure cluster object bucket name.
	BucketName() string
	// Cache returns the reconciler cache which can be used for instance to cache values across AWS SDK clients
	// and sessions.
	Cache() *gocache.Cache
	// Cluster returns the AWS infrastructure cluster.
	Cluster() runtime.Object
	// Cluster returns the AWS infrastructure cluster name.
	ClusterName() string
	// Cluster returns the AWS infrastructure cluster namespace.
	ClusterNamespace() string
	// Installation returns the installation name.
	Installation() string
	// MigrationNeeded checks if cluster needs migrated first.
	MigrationNeeded() bool
	// Region returns the AWS infrastructure cluster object region.
	Region() string
	// CloudFormation Caller Reference
	CallerReference() string
}
