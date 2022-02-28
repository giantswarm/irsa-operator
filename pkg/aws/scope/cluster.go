package scope

import (
	awsclient "github.com/aws/aws-sdk-go/aws/client"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/klogr"
)

// ClusterScopeParams defines the input parameters used to create a new Scope.
type ClusterScopeParams struct {
	ARN              string
	AccountID        string
	Cluster          runtime.Object
	ClusterName      string
	ClusterNamespace string
	BucketName       string
	Region           string

	Logger  logr.Logger
	Session awsclient.ConfigProvider
}

// NewClusterScope creates a new Scope from the supplied parameters.
// This is meant to be called for each reconcile iteration.
func NewClusterScope(params ClusterScopeParams) (*ClusterScope, error) {
	if params.ARN == "" {
		return nil, errors.New("failed to generate new scope from emtpy string ARN")
	}
	if params.AccountID == "" {
		return nil, errors.New("failed to generate new scope from emtpy string AccountID")
	}
	if params.Cluster == nil {
		return nil, errors.New("failed to generate new scope from nil Cluster")
	}
	if params.ClusterName == "" {
		return nil, errors.New("failed to generate new scope from emtpy string ClusterName")
	}
	if params.ClusterNamespace == "" {
		return nil, errors.New("failed to generate new scope from emtpy string ClusterNamespace")
	}
	if params.BucketName == "" {
		return nil, errors.New("failed to generate new scope from emtpy string BucketName")
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
		assumeRole:       params.ARN,
		accountID:        params.AccountID,
		cluster:          params.Cluster,
		clusterName:      params.ClusterName,
		clusterNamespace: params.ClusterNamespace,
		bucketName:       params.BucketName,
		region:           params.Region,

		Logger:  params.Logger,
		session: session,
	}, nil
}

// ClusterScope defines the basic context for an actuator to operate upon.
type ClusterScope struct {
	assumeRole       string
	accountID        string
	cluster          runtime.Object
	clusterName      string
	clusterNamespace string
	bucketName       string
	region           string

	logr.Logger
	session awsclient.ConfigProvider
}

// ARN returns the AWS SDK assumed role.
func (s *ClusterScope) ARN() string {
	return s.assumeRole
}

// Account ID returns the account ID of the assumed role.
func (s *ClusterScope) AccountID() string {
	return s.accountID
}

// BucketName returns the name of the OIDC S3 bucket.
func (s *ClusterScope) BucketName() string {
	return s.bucketName
}

// Cluster returns the AWS infrastructure cluster object.
func (s *ClusterScope) Cluster() runtime.Object {
	return s.cluster
}

// ClusterName returns the name of AWS infrastructure cluster object.
func (s *ClusterScope) ClusterName() string {
	return s.clusterName
}

// ClusterNameSpace returns the namespace of AWS infrastructure cluster object.
func (s *ClusterScope) ClusterNamespace() string {
	return s.clusterNamespace
}
func (s *ClusterScope) Region() string {
	return s.region
}

// Session returns the AWS SDK session.
func (s *ClusterScope) Session() awsclient.ConfigProvider {
	return s.session
}
