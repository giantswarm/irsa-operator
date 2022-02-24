package key

const (
	ClusterNameLabel     = "cluster.x-k8s.io/cluster-name"
	CAPIWatchFilterLabel = "cluster.x-k8s.io/watch-filter"
	CAPAReleaseComponent = "cluster-api-provider-aws"
	FinalizerName        = "irsa-operator.finalizers.giantswarm.io"
	//TODO move it into k8smetadata
	IRSAAnnotation = "alpha.aws.giantswarm.io/iam-roles-for-service-accounts"
)
