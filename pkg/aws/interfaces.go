package aws

import (
	awsclient "github.com/aws/aws-sdk-go/aws/client"
	"github.com/go-logr/logr"
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
	// BucketName returns the AWS infrastructure cluster object bucket name.
	BucketName() string
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
}
