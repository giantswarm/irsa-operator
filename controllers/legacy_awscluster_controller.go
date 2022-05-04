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
	"fmt"
	"regexp"
	"time"

	"github.com/giantswarm/apiextensions/v6/pkg/apis/infrastructure/v1alpha3"
	infrastructurev1alpha3 "github.com/giantswarm/apiextensions/v6/pkg/apis/infrastructure/v1alpha3"
	"github.com/giantswarm/microerror"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/giantswarm/irsa-operator/pkg/aws/scope"
	"github.com/giantswarm/irsa-operator/pkg/irsa"
	"github.com/giantswarm/irsa-operator/pkg/key"
)

// LegacyClusterReconciler reconciles a Giant Swarm AWSCluster object
type LegacyClusterReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	Installation string
	recorder     record.EventRecorder
}

// +kubebuilder:rbac:groups=infrastructure.giantswarm.io,resources=awscluster,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.giantswarm.io,resources=awscluster/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.giantswarm.io,resources=awscluster/finalizers,verbs=update

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

	if _, ok := cluster.Annotations[key.IRSAAnnotation]; !ok {
		logger.Info(fmt.Sprintf("AWSCluster CR do not have required annotation '%s' , ignoring CR", key.IRSAAnnotation))
		// resource does not contain IRSA annotation, try later
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: time.Minute * 5,
		}, nil
	}

	// fetch ARN from the cluster to assume role for creating dependencies
	credentialName := cluster.Spec.Provider.CredentialSecret.Name
	credentialNamespace := cluster.Spec.Provider.CredentialSecret.Namespace
	var credentialSecret = &v1.Secret{}
	if err = r.Get(ctx, types.NamespacedName{Namespace: credentialNamespace, Name: credentialName}, credentialSecret); err != nil {
		logger.Error(err, "failed to get credential secret")
		return ctrl.Result{}, microerror.Mask(err)
	}

	secretByte, ok := credentialSecret.Data["aws.awsoperator.arn"]
	if !ok {
		logger.Error(err, "Unable to extract ARN from secret")
		return ctrl.Result{}, microerror.Mask(fmt.Errorf("Unable to extract ARN from secret %s for cluster %s", credentialName, cluster.Name))

	}

	// convert secret data secretByte into string
	arn := string(secretByte)

	// extract AccountID from ARN
	re := regexp.MustCompile(`[-]?\d[\d,]*[\.]?[\d{2}]*`)
	accountID := re.FindAllString(arn, 1)[0]

	if accountID == "" {
		logger.Error(err, "Unable to extract Account ID from ARN")
		return ctrl.Result{}, microerror.Mask(fmt.Errorf("Unable to extract Account ID from ARN %s", string(arn)))

	}

	// create the cluster scope.
	clusterScope, err := scope.NewClusterScope(scope.ClusterScopeParams{
		AccountID:        accountID,
		ARN:              arn,
		BucketName:       key.BucketName(accountID, cluster.Name),
		ClusterName:      cluster.Name,
		ClusterNamespace: cluster.Namespace,
		Installation:     r.Installation,
		Region:           cluster.Spec.Provider.Region,
		SecretName:       key.SecretName(cluster.Name),

		Logger:  logger,
		Cluster: cluster,
	})
	if err != nil {
		return reconcile.Result{}, microerror.Mask(err)
	}

	// Create IRSA service.
	irsaService := irsa.New(clusterScope, r.Client)

	if cluster.DeletionTimestamp != nil {
		err := irsaService.Delete(ctx)
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}

		if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
			logger.Error(err, "Cluster does not exist")
			return ctrl.Result{}, microerror.Mask(err)
		}

		controllerutil.RemoveFinalizer(cluster, key.FinalizerName)
		err = r.Update(ctx, cluster)
		if err != nil {
			logger.Error(err, "failed to remove finalizer on AWSCluster CR")
			return ctrl.Result{}, microerror.Mask(err)
		}
		r.sendEvent(cluster, v1.EventTypeNormal, "IRSA", "IRSA bootstrap deleted")

		return ctrl.Result{}, nil

	} else {
		err := irsaService.Reconcile(ctx)
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}
		if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
			logger.Error(err, "Cluster does not exist")
			return ctrl.Result{}, microerror.Mask(err)
		}

		controllerutil.AddFinalizer(cluster, key.FinalizerName)
		err = r.Update(ctx, cluster)
		if err != nil {
			logger.Error(err, "failed to add finalizer on AWSCluster CR")
			return ctrl.Result{}, microerror.Mask(err)
		}
		r.sendEvent(cluster, v1.EventTypeNormal, "IRSA", "IRSA bootstrap created")
	}

	return ctrl.Result{
		Requeue:      true,
		RequeueAfter: time.Minute * 5,
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LegacyClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha3.AWSCluster{}).
		Complete(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.recorder = mgr.GetEventRecorderFor("legacy-controller")
	return nil
}

func (r *LegacyClusterReconciler) sendEvent(cluster *v1alpha3.AWSCluster, eventtype, reason, message string) {
	r.recorder.Event(cluster, eventtype, reason, message)
}
