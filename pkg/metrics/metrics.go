package controllers

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	metricNamespace = "irsa_operator"
	metricSubsystem = "cluster"

	labelAccountID    = "account_id"
	labelCluster      = "cluster_id"
	labelNamespace    = "cluster_namespace"
	labelInstallation = "installation"
)

var (
	labels = []string{labelInstallation, labelAccountID, labelCluster, labelNamespace}

	Errors = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Subsystem: metricSubsystem,
			Name:      "errors",
			Help:      "Number of errors",
		},
		labels,
	)
)

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(Errors)
}
