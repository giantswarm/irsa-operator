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
	logr.Logger
	Session

	// ARN returns the workload cluster assumed role to operate.
	ARN() string
	// InfraCluster returns the AWS infrastructure cluster.
	InfraCluster() runtime.Object
	// Region returns the AWS infrastructure cluster object region.
	Region() string
}
