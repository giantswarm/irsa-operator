package scope

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	awsclient "github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/blang/semver"
	"github.com/go-logr/logr"
	gocache "github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/giantswarm/irsa-operator/pkg/key"
)

// ClusterScopeParams defines the input parameters used to create a new Scope.
type ClusterScopeParams struct {
	AccountID                   string
	ARN                         string
	BaseDomain                  string
	BucketName                  string
	Cache                       *gocache.Cache
	Cluster                     runtime.Object
	ClusterName                 string
	ClusterNamespace            string
	ConfigName                  string
	Installation                string
	KeepCloudFrontOIDCProvider  bool
	ManagementClusterAccountID  string
	ManagementClusterIAMRoleArn string
	Migration                   bool
	PreCloudfrontAlias          bool
	Region                      string
	ReleaseVersion              string
	SecretName                  string
	VPCMode                     string

	Logger  logr.Logger
	Session awsclient.ConfigProvider
}

// NewClusterScope creates a new Scope from the supplied parameters.
// This is meant to be called for each reconcile iteration.
func NewClusterScope(params ClusterScopeParams) (*ClusterScope, error) {
	if params.AccountID == "" {
		return nil, errors.New("failed to generate new scope from emtpy string AccountID")
	}
	if params.ARN == "" {
		return nil, errors.New("failed to generate new scope from emtpy string ARN")
	}
	if params.BucketName == "" {
		return nil, errors.New("failed to generate new scope from emtpy string BucketName")
	}
	if params.Cache == nil {
		return nil, errors.New("failed to generate new scope from nil Cache")
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
	if params.ConfigName == "" {
		return nil, errors.New("failed to generate new scope from emtpy string ConfigName")
	}
	if params.Installation == "" {
		return nil, errors.New("failed to generate new scope from emtpy string Installation")
	}
	if params.Region == "" {
		return nil, errors.New("failed to generate new scope from emtpy string Region")
	}
	if params.ReleaseVersion == "" {
		return nil, errors.New("failed to generate new scope from emtpy string ReleaseVersion")
	}
	if params.SecretName == "" {
		return nil, errors.New("failed to generate new scope from emtpy string SecretName")
	}

	// `ParseTolerant` instead of `Parse` in case we ever mistakenly use the `v` version prefix or other non-strict format
	releaseSemver, err := semver.ParseTolerant(params.ReleaseVersion)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse release version %q", params.ReleaseVersion)
	}

	session, err := sessionForRegion(params.Region)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create aws session")
	}

	awsClientConfig := &aws.Config{Credentials: stscreds.NewCredentials(session, params.ARN)}

	stsClient := sts.New(session, awsClientConfig)
	o, err := stsClient.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get sts client")
	}

	params.Logger.Info(fmt.Sprintf("assumed role %s", *o.Arn))

	return &ClusterScope{
		accountID:                   params.AccountID,
		managementClusterAccountID:  params.ManagementClusterAccountID,
		managementClusterIAMRoleArn: params.ManagementClusterIAMRoleArn,
		workloadClusterIAMRoleArn:   params.ARN,
		baseDomain:                  params.BaseDomain,
		bucketName:                  params.BucketName,
		cache:                       params.Cache,
		cluster:                     params.Cluster,
		clusterName:                 params.ClusterName,
		clusterNamespace:            params.ClusterNamespace,
		configName:                  params.ConfigName,
		installation:                params.Installation,
		keepCloudFrontOIDCProvider:  params.KeepCloudFrontOIDCProvider,
		migration:                   params.Migration,
		preCloudfrontAlias:          params.PreCloudfrontAlias,
		region:                      params.Region,
		releaseVersion:              params.ReleaseVersion,
		releaseSemver:               releaseSemver,
		secretName:                  params.SecretName,
		vpcMode:                     params.VPCMode,

		Logr:    params.Logger,
		session: session,
	}, nil
}

// ClusterScope defines the basic context for an actuator to operate upon.
type ClusterScope struct {
	accountID                   string
	baseDomain                  string
	bucketName                  string
	workloadClusterIAMRoleArn   string
	cache                       *gocache.Cache
	cluster                     runtime.Object
	clusterName                 string
	clusterNamespace            string
	configName                  string
	installation                string
	keepCloudFrontOIDCProvider  bool
	managementClusterAccountID  string
	managementClusterIAMRoleArn string
	migration                   bool
	preCloudfrontAlias          bool
	region                      string
	releaseVersion              string
	releaseSemver               semver.Version
	secretName                  string
	vpcMode                     string

	Logr    logr.Logger
	session awsclient.ConfigProvider
}

func (s *ClusterScope) Logger() logr.Logger {
	return s.Logr
}

// AccountID returns the account ID of the assumed role.
func (s *ClusterScope) AccountID() string {
	return s.accountID
}

// ManagementClusterAccountID returns the account ID used by the Management Cluster.
func (s *ClusterScope) ManagementClusterAccountID() string {
	return s.managementClusterAccountID
}

// ARN returns the AWS SDK assumed role.
func (s *ClusterScope) ARN() string {
	return s.workloadClusterIAMRoleArn
}

// ManagementClusterIAMRoleArn returns the IAM Role to assume when changing resources in the MC account.
func (s *ClusterScope) ManagementClusterIAMRoleArn() string {
	return s.managementClusterIAMRoleArn
}

// BaseDomain returns the cluster DNS zone.
func (s *ClusterScope) BaseDomain() string {
	return s.baseDomain
}

// BucketName returns the name of the OIDC S3 bucket.
func (s *ClusterScope) BucketName() string {
	if key.IsCAPARelease(s.Release()) {
		return fmt.Sprintf("%s-v3", s.bucketName)
	} else if key.IsV18Release(s.Release()) || s.MigrationNeeded() {
		return fmt.Sprintf("%s-v2", s.bucketName)
	} else {
		return s.bucketName
	}
}

func (s *ClusterScope) Cache() *gocache.Cache {
	return s.cache
}

// Cluster returns the AWS infrastructure cluster object.
func (s *ClusterScope) Cluster() runtime.Object {
	return s.cluster
}

// ClusterName returns the name of AWS infrastructure cluster object.
func (s *ClusterScope) ClusterName() string {
	return s.clusterName
}

func (s *ClusterScope) CallerReference() string {
	if key.IsCAPARelease(s.Release()) {
		return fmt.Sprintf("distribution-cluster-%s-capa", s.clusterName)
	} else {
		return fmt.Sprintf("distribution-cluster-%s", s.clusterName)
	}
}

// ClusterNameSpace returns the namespace of AWS infrastructure cluster object.
func (s *ClusterScope) ClusterNamespace() string {
	return s.clusterNamespace
}

// ConfigName returns the name of Cloudfront config from the cluster.
func (s *ClusterScope) ConfigName() string {
	return s.configName
}

// Installation returns the name of the installation where the cluster object is located.
func (s *ClusterScope) Installation() string {
	return s.installation
}

// KeepCloudFrontOIDCProvider returns whether the `<random>.cloudfront.net` OIDC provider
// domain should be created/kept (true) or deleted (false)
func (s *ClusterScope) KeepCloudFrontOIDCProvider() bool {
	return s.keepCloudFrontOIDCProvider
}

// MigrationNeeded returns if the cluster object needs migration beforehand.
func (s *ClusterScope) MigrationNeeded() bool {
	return s.migration
}

// PreCloudfrontAlias returns if the cloudfront alias should be used before v19.0.0.
func (s *ClusterScope) PreCloudfrontAlias() bool {
	return s.preCloudfrontAlias
}

// Region returns the region of the AWS infrastructure cluster object.
func (s *ClusterScope) Region() string {
	return s.region
}

// ReleaseVersion returns the release version of the AWS cluster object.
func (s *ClusterScope) ReleaseVersion() string {
	return s.releaseVersion
}

// Release returns the semver version of the AWS cluster object.
func (s *ClusterScope) Release() *semver.Version {
	return &s.releaseSemver
}

// SecretName returns the name of the OIDC secret from the cluster.
func (s *ClusterScope) SecretName() string {
	return s.secretName
}

// Session returns the AWS SDK session.
func (s *ClusterScope) Session() awsclient.ConfigProvider {
	return s.session
}

// VPCMode returns the VPC mode used on this cluster.
func (s *ClusterScope) VPCMode() string {
	return s.vpcMode
}
