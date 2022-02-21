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

package controllers

import (
	"context"
	"time"

	"github.com/blang/semver"
	infrastructurev1alpha3 "github.com/giantswarm/apiextensions/v3/pkg/apis/infrastructure/v1alpha3"
	"github.com/giantswarm/k8smetadata/pkg/label"
	"github.com/giantswarm/microerror"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// LegacyClusterReconciler reconciles a Giant Swarm AWSCluster object
type LegacyClusterReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=infrastructure.giantswarm.io,resources=awscluster,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.giantswarm.io,resources=awscluster/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.giantswarm.io,resources=awscluster/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *LegacyClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var err error
	logger := r.Log.WithValues("namespace", req.Namespace, "cluster", req.Name)

	cluster := &infrastructurev1alpha3.AWSCluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		logger.Error(err, "Cluster does not exist")
		return ctrl.Result{}, microerror.Mask(err)
	}

	// if the cluster CR has a old GS release label we check if the release version is old enought for encryption operator,
	// otherwise ignore the CR
	if v, ok := cluster.Labels[label.ReleaseVersion]; ok {
		_, err := semver.Parse(v)
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}

	} else {
		logger.Info("did not found release label on cluster CR, assuming CAPI release")
	}

	//var irsaService *irsa.Service
	{
		//c := encryption.Config{
		//	AppCatalog:               r.AppCatalog,
		//	Cluster:                  cluster,
		//	CtrlClient:               r.Client,
		//	DefaultKeyRotationPeriod: r.DefaultKeyRotationPeriod,
		//	RegistryDomain:           r.RegistryDomain,
		//	Logger:                   logger,
		//}

		//encryptionService, err = encryption.New(c)
		//if err != nil {
		//	logger.Error(err, "failed to create encryption service")
		//	return ctrl.Result{}, microerror.Mask(err)
		//}
	}

	if cluster.DeletionTimestamp != nil {
		// clean
		//err = encryptionService.Delete()
		//if err != nil {
		//	logger.Error(err, "failed to clean resources")
		//	return ctrl.Result{}, microerror.Mask(err)
		//}
		// remove finalizer from Cluster
		controllerutil.RemoveFinalizer(cluster, "")
		err = r.Update(ctx, cluster)
		if err != nil {
			logger.Error(err, "failed to remove finalizer on Cluster CR")
			return ctrl.Result{}, microerror.Mask(err)
		}
		// resource was cleaned up, we dont need to reconcile again
		return ctrl.Result{}, nil

	} else {
		// reconcile
		//err = encryptionService.Reconcile()
		//if err != nil {
		//	logger.Error(err, "failed to reconcile resource")
		//	return ctrl.Result{}, microerror.Mask(err)
		//}

		// add finalizer to AWSMachineTemplate
		controllerutil.AddFinalizer(cluster, "")
		err = r.Update(ctx, cluster)
		if err != nil {
			logger.Error(err, "failed to add finalizer on Cluster CR")
			return ctrl.Result{}, microerror.Mask(err)
		}
	}

	return ctrl.Result{
		Requeue:      true,
		RequeueAfter: time.Minute * 5,
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LegacyClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha3.AWSCluster{}).
		Complete(r)
}
