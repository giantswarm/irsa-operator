package key

import (
	"fmt"
	"strings"

	"github.com/blang/semver"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
)

const (
	ClusterNameLabel     = "cluster.x-k8s.io/cluster-name"
	CAPIWatchFilterLabel = "cluster.x-k8s.io/watch-filter"
	CAPAReleaseComponent = "cluster-api-provider-aws"
	FinalizerName        = "irsa-operator.finalizers.giantswarm.io"
	//TODO move it into k8smetadata
	IRSAAnnotation = "alpha.aws.giantswarm.io/iam-roles-for-service-accounts"
	// Upgrading existing IRSA clusters witout breaking clusters
	IRSAMigrationAnnotation = "alpha.aws.giantswarm.io/irsa-migration"

	S3TagCloudProvider = "kubernetes.io/cluster/%s"
	S3TagCluster       = "giantswarm.io/cluster"
	S3TagInstallation  = "giantswarm.io/installation"
	S3TagOrganization  = "giantswarm.io/organization"

	CustomerTagLabel = "tag.provider.giantswarm.io/"
	ReleaseLabel     = "release.giantswarm.io/version"

	V18AlphaRelease = "18.0.0-alpha1"
	V19AlphaRelease = "19.0.0-alpha1"
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

func AWSEndpoint(region string) string {
	awsEndpoint := "amazonaws.com"
	if strings.HasPrefix(region, "cn-") {
		awsEndpoint = "amazonaws.com.cn"
	}
	return awsEndpoint
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
	v18AlphaVersion, _ := semver.New(V18AlphaRelease)
	return releaseVersion.GE(*v18AlphaVersion)
}

func IsV19Release(releaseVersion *semver.Version) bool {
	v19, _ := semver.New(V19AlphaRelease)
	return releaseVersion.GE(*v19)
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
