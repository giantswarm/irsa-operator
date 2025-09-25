/*
Copyright 2022.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"os"
	"time"

	infrastructurev1alpha3 "github.com/giantswarm/apiextensions/v6/pkg/apis/infrastructure/v1alpha3"
	gocache "github.com/patrickmn/go-cache"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	capa "sigs.k8s.io/cluster-api-provider-aws/v2/api/v1beta2"
	eks "sigs.k8s.io/cluster-api-provider-aws/v2/controlplane/eks/api/v1beta2"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/giantswarm/irsa-operator/controllers"
	"github.com/giantswarm/irsa-operator/pkg/aws/scope"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = capa.AddToScheme(scheme)
	_ = capi.AddToScheme(scheme)
	_ = eks.AddToScheme(scheme)
	_ = infrastructurev1alpha3.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var capa bool
	var legacy bool
	var probeAddr string
	var installation string
	var maxConcurrentReconciles int

	flag.BoolVar(&capa, "capa", false, "Reconciles on CAPA resources.")
	flag.BoolVar(&legacy, "legacy", false, "Reconciles on GiantSwarm AWS resources.")
	flag.StringVar(&installation, "installation", "", "The name of the installation.")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.IntVar(&maxConcurrentReconciles, "max-concurrent-reconciles", 4, "The maximum number of concurrent reconciles for the controller.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: false,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		WebhookServer: webhook.NewServer(
			webhook.Options{
				Port: 9443,
			},
		),
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "r468hqyb4.giantswarm.io",
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: []client.Object{
					&corev1.ConfigMap{},
					&corev1.Secret{},
				},
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	cache := gocache.New(
		// The cache is shared and can be used for various things.
		// A reasonable expiration duration should be specified at usage, not here.
		1*time.Second,

		15*time.Second)

	if legacy {
		if err = (&controllers.LegacyClusterReconciler{
			Client:       mgr.GetClient(),
			Log:          ctrl.Log.WithName("legacy-controller"),
			Scheme:       mgr.GetScheme(),
			Installation: installation,
			Cache:        cache,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Cluster")
			os.Exit(1)
		}
	}

	if capa {
		opts := controller.Options{
			MaxConcurrentReconciles: maxConcurrentReconciles,
		}
		if err = (&controllers.CAPAClusterReconciler{
			Client:       mgr.GetClient(),
			Log:          ctrl.Log.WithName("capa-controller"),
			Scheme:       mgr.GetScheme(),
			Installation: installation,
			Cache:        cache,
		}).SetupWithManager(mgr, opts); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Cluster")
			os.Exit(1)
		}
		if err = (&controllers.EKSClusterReconciler{
			Client:       mgr.GetClient(),
			Log:          ctrl.Log.WithName("eks-controller"),
			Scheme:       mgr.GetScheme(),
			Installation: installation,
			Cache:        cache,
		}).SetupWithManager(mgr, opts); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "AWSManagedControlPlane")
			os.Exit(1)
		}

	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager", "currentCommit", scope.CurrentCommit)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
