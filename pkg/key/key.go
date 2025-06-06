package key

import (
	"fmt"
	"strings"

	"github.com/blang/semver"
	"github.com/giantswarm/microerror"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
)

const (
	FinalizerNameDeprecated = "irsa-operator.finalizers.giantswarm.io" // should go away since it's not fully qualified
	FinalizerName           = "irsa-operator.finalizers.giantswarm.io/resource"
	// TODO move it into k8smetadata
	IRSAAnnotation = "alpha.aws.giantswarm.io/iam-roles-for-service-accounts"
	// Upgrading existing IRSA clusters witout breaking clusters
	IRSAMigrationAnnotation = "alpha.aws.giantswarm.io/irsa-migration"
	// Use Cloudfront alias before v19.0.0
	IRSAPreCloudfrontAliasAnnotation = "alpha.aws.giantswarm.io/enable-cloudfront-alias"
	// Keep IRSA label
	KeepIRSALabel = "giantswarm.io/keep-irsa"
	// Pause IRSA operator
	PauseIRSAOperatorAnnotation = "giantswarm.io/pause-irsa-operator"
	// Whether to create/keep the `<random>.cloudfront.net` OIDC provider. Only used for vintage. Defaults
	// to `true` for backward compatibility, and only the values `true` or `false` are allowed.
	// If a single cluster doesn't have any IAM roles using the `<random>.cloudfront.net` OIDC provider domain,
	// this annotation can be set to `false` in order to make the operator delete that OIDC provider.
	// The CloudFront distribution is of course not deleted, since it also hosts the OIDC configuration for the
	// predictable `irsa.<basedomain>` OIDC provider (which customers should use).
	KeepCloudFrontOIDCProviderAnnotation = "alpha.aws.giantswarm.io/irsa-keep-cloudfront-oidc-provider"

	S3TagCloudProvider = "kubernetes.io/cluster/%s"
	S3TagCluster       = "giantswarm.io/cluster"
	S3TagInstallation  = "giantswarm.io/installation"
	S3TagOrganization  = "giantswarm.io/organization"

	CustomerTagLabel = "tag.provider.giantswarm.io/"
	ReleaseLabel     = "release.giantswarm.io/version"
)

func BucketName(accountID, clusterName string) string {
	return fmt.Sprintf("%s-g8s-%s-oidc-pod-identity", accountID, clusterName)
}

func ConfigName(clusterName string) string {
	return fmt.Sprintf("%s-irsa-cloudfront", clusterName)
}

func SecretName(clusterName string) string {
	return fmt.Sprintf("%s-service-account-v2", clusterName)
}

func Release(getter LabelsGetter) string {
	return getter.GetLabels()[ReleaseLabel]
}

func KeepOnDeletion(getter LabelsGetter) bool {
	_, ok := getter.GetLabels()[KeepIRSALabel]
	return ok
}

func AWSEndpoint(region string) string {
	awsEndpoint := "amazonaws.com"
	if strings.HasPrefix(region, "cn-") {
		awsEndpoint = "amazonaws.com.cn"
	}
	return awsEndpoint
}

func STSUrl(region string) string {
	return fmt.Sprintf("sts.%s", AWSEndpoint(region))
}

func IsChina(region string) bool {
	return strings.HasPrefix(region, "cn-")
}

func ARNPrefix(region string) string {
	arnPrefix := "aws"
	if strings.HasPrefix(region, "cn-") {
		arnPrefix = "aws-cn"
	}
	return arnPrefix
}

func IsV18Release(releaseVersion *semver.Version) bool {
	return releaseVersion.Major >= 18
}

func IsV19Release(releaseVersion *semver.Version) bool {
	return releaseVersion.Major >= 19
}

func IsCAPARelease(releaseVersion *semver.Version) bool {
	return releaseVersion.Major >= 25
}

func ContainsFinalizer(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}

func GetCustomerTags(cluster *capi.Cluster) map[string]string {
	customerTags := make(map[string]string)

	for k, v := range cluster.Labels {
		if strings.HasPrefix(k, CustomerTagLabel) {
			customerTags[strings.Replace(k, CustomerTagLabel, "", 1)] = v
		}
	}
	return customerTags
}

func CloudFrontDistributionComment(clusterID string) string {
	return fmt.Sprintf("Created by irsa-operator for cluster %s", clusterID)
}

func CloudFrontAlias(baseDomain string) string {
	return fmt.Sprintf("irsa.%s", baseDomain)
}

func BaseDomain(cluster capi.Cluster) (string, error) {
	apiEndpoint := cluster.Spec.ControlPlaneEndpoint.Host
	if apiEndpoint == "" {
		return "", microerror.Mask(missingApiEndpointError)
	}
	if !strings.HasPrefix(apiEndpoint, "api.") {
		return "", microerror.Mask(unexpectedApiEndpointError)
	}
	return strings.TrimPrefix(apiEndpoint, "api."), nil
}

func EnsureTrailingDot(domain string) string {
	if strings.HasSuffix(domain, ".") {
		return domain
	}

	return fmt.Sprintf("%s.", domain)
}
