package controllers

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	metricNamespace               = "irsa_operator"
	errorMetricSubsystem          = "cluster"
	acmCertificateMetricSubsystem = "acm_certificate"

	labelAccountID       = "account_id"
	labelCertificateName = "certificate_name"
	labelCluster         = "cluster_id"
	labelNamespace       = "cluster_namespace"
	labelInstallation    = "installation"
)

var (
	commonLabels = []string{labelInstallation, labelAccountID, labelCluster, labelNamespace}

	Errors = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Subsystem: errorMetricSubsystem,
			Name:      "errors",
			Help:      "Number of errors",
		},
		commonLabels,
	)

	Certs = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Subsystem: acmCertificateMetricSubsystem,
			Name:      "not_after",
			Help:      "Expiration date of ACM certificates used for IRSA",
		},
		append(commonLabels, labelCertificateName),
	)
)

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(Errors)
	metrics.Registry.MustRegister(Certs)
}
