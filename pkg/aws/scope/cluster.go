package scope

import (
	awsclient "github.com/aws/aws-sdk-go/aws/client"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/klog/klogr"
)

// ClusterScopeParams defines the input parameters used to create a new Scope.
type ClusterScopeParams struct {
	ARN        string
	AWSCluster string
	Region     string
	Logger     logr.Logger
	Session    awsclient.ConfigProvider
}

// NewClusterScope creates a new Scope from the supplied parameters.
// This is meant to be called for each reconcile iteration.
func NewClusterScope(params ClusterScopeParams) (*ClusterScope, error) {
	if params.ARN == "" {
		return nil, errors.New("failed to generate new scope from emtpy string ARN")
	}
	if params.AWSCluster == "" {
		return nil, errors.New("failed to generate new scope from empty string AWSCluster")
	}
	if params.Region == "" {
		return nil, errors.New("failed to generate new scope from emtpy string Region")
	}
	if params.Logger == nil {
		params.Logger = klogr.New()
	}

	session, err := sessionForRegion(params.Region)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create aws session")
	}

	return &ClusterScope{
		assumeRole: params.ARN,
		awsCluster: params.AWSCluster,
		Logger:     params.Logger,
		session:    session,
	}, nil
}

// ClusterScope defines the basic context for an actuator to operate upon.
type ClusterScope struct {
	assumeRole string
	awsCluster string
	logr.Logger
	session awsclient.ConfigProvider
}

// ARN returns the AWS SDK assumed role.
func (s *ClusterScope) ARN() string {
	return s.assumeRole
}

// InfraClusterCluster returns the name of the AWS infrastructure cluster.
func (s *ClusterScope) InfraClusterName() string {
	return s.awsCluster
}

// Session returns the AWS SDK session.
func (s *ClusterScope) Session() awsclient.ConfigProvider {
	return s.session
}
