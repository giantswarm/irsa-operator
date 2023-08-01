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

	"github.com/giantswarm/microerror"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	eks "sigs.k8s.io/cluster-api-provider-aws/controlplane/eks/api/v1beta1"
	capa "sigs.k8s.io/cluster-api-provider-aws/v2/api/v1beta2"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/giantswarm/irsa-operator/pkg/aws/scope"
	irsaCapa "github.com/giantswarm/irsa-operator/pkg/irsa/capa"
	"github.com/giantswarm/irsa-operator/pkg/key"
)

// EKSClusterReconciler reconciles a CAPA AWSManagedCluster object
type EKSClusterReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	Installation string
	recorder     record.EventRecorder
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=awsmanagedcluster,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=awsmanagedcluster/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=awsmanagedcluster/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *EKSClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var err error
	logger := r.Log.WithValues("namespace", req.Namespace, "cluster", req.Name)

	logger.Info("Reconciling AWSManagedCluster for EKS")

	cluster := &capa.AWSManagedCluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		return ctrl.Result{}, microerror.Mask(client.IgnoreNotFound(err))
	}

	eksCluster := &eks.AWSManagedControlPlane{}
	if err := r.Get(ctx, req.NamespacedName, eksCluster); err != nil {
		return ctrl.Result{}, microerror.Mask(client.IgnoreNotFound(err))
	}

	awsClusterRoleIdentity := &capa.AWSClusterRoleIdentity{}
	err = r.Get(ctx, types.NamespacedName{Name: eksCluster.Spec.IdentityRef.Name}, awsClusterRoleIdentity)
	if err != nil {
		return ctrl.Result{}, microerror.Mask(fmt.Errorf("failed to get AWSClusterRoleIdentity object %q: %w", eksCluster.Spec.IdentityRef.Name, err))
	}

	arn := awsClusterRoleIdentity.Spec.RoleArn

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
		ConfigName:       key.ConfigName(cluster.Name),
		Installation:     r.Installation,
		Region:           eksCluster.Spec.Region,
		// This is a hack to allow CAPI clusters to drop the 'release.giantswarm.io/version' label.
		ReleaseVersion: "20.0.0-alpha1",
		SecretName:     key.SecretName(cluster.Name),

		Logger:  logger,
		Cluster: cluster,
	})
	if err != nil {
		return reconcile.Result{}, microerror.Mask(err)
	}

	// Create IRSA service.
	irsaService := irsaCapa.New(clusterScope, r.Client)

	if cluster.DeletionTimestamp != nil {
		finalizers := cluster.GetFinalizers()
		if !key.ContainsFinalizer(finalizers, key.FinalizerName) && !key.ContainsFinalizer(finalizers, key.FinalizerNameDeprecated) {
			return ctrl.Result{}, nil
		}

		err := irsaService.Delete(ctx)
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}

		patchHelper, err := patch.NewHelper(cluster, r.Client)
		if err != nil {
			return ctrl.Result{}, err
		}
		controllerutil.RemoveFinalizer(cluster, key.FinalizerName)
		err = patchHelper.Patch(ctx, cluster)
		if err != nil {
			logger.Error(err, "failed to remove finalizer from AWSManagedCluster")
			return ctrl.Result{}, err
		}
		logger.Info("successfully removed finalizer from AWSManagedCluster")

		r.sendEvent(cluster, v1.EventTypeNormal, "IRSA", "IRSA bootstrap deleted")

		return ctrl.Result{}, nil
	} else {
		created := false
		if !controllerutil.ContainsFinalizer(cluster, key.FinalizerName) {
			created = true

			patchHelper, err := patch.NewHelper(cluster, r.Client)
			if err != nil {
				return ctrl.Result{}, microerror.Mask(err)
			}
			controllerutil.AddFinalizer(cluster, key.FinalizerName)
			err = patchHelper.Patch(ctx, cluster)
			if err != nil {
				logger.Error(err, "failed to add finalizer on AWSManagedCluster")
				return ctrl.Result{}, microerror.Mask(err)
			}
			logger.Info("successfully added finalizer to AWSManagedCluster")
		}

		err := irsaService.Reconcile(ctx)
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}

		if created {
			r.sendEvent(cluster, v1.EventTypeNormal, "IRSA", "IRSA bootstrap created")
		}

		// Re-run regularly to ensure OIDC certificate thumbprints are up to date (see `EnsureOIDCProviders`)
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: time.Minute * 5,
		}, nil
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *EKSClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&capa.AWSManagedCluster{}).
		Complete(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.recorder = mgr.GetEventRecorderFor("irsa-eks-controller")
	return nil
}

func (r *EKSClusterReconciler) sendEvent(cluster *capa.AWSManagedCluster, eventtype, reason, message string) {
	r.recorder.Eventf(cluster, v1.EventTypeNormal, reason, message)
}
