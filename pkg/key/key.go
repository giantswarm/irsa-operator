package key

import "fmt"

const (
	ClusterNameLabel     = "cluster.x-k8s.io/cluster-name"
	CAPIWatchFilterLabel = "cluster.x-k8s.io/watch-filter"
	CAPAReleaseComponent = "cluster-api-provider-aws"
	FinalizerName        = "irsa-operator.finalizers.giantswarm.io"
	//TODO move it into k8smetadata
	IRSAAnnotation = "alpha.aws.giantswarm.io/iam-roles-for-service-accounts"

	S3TagCloudProvider = "kubernetes.io/cluster/%s"
	S3TagCluster       = "giantswarm.io/cluster"
	S3TagInstallation  = "giantswarm.io/installation"
	S3TagOrganization  = "giantswarm.io/organization"

	CustomerTagLabel = "tag.provider.giantswarm.io"
)

func BucketName(accountID, clusterName string) string {
	return fmt.Sprintf("%s-g8s-%s-oidc-pod-identity", accountID, clusterName)
}

func SecretName(clusterName string) string {
	return fmt.Sprintf("%s-service-account-v2", clusterName)
}
