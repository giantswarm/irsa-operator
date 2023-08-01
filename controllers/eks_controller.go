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

	"github.com/giantswarm/microerror"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	capa "sigs.k8s.io/cluster-api-provider-aws/v2/api/v1beta2"
	eks "sigs.k8s.io/cluster-api-provider-aws/v2/controlplane/eks/api/v1beta2"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/giantswarm/irsa-operator/pkg/aws/scope"
	irsaEks "github.com/giantswarm/irsa-operator/pkg/irsa/eks"
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

func (r *EKSClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var err error
	logger := r.Log.WithValues("namespace", req.Namespace, "cluster", req.Name)

	logger.Info("Reconciling AWSManagedControlPlane")

	eksCluster := &eks.AWSManagedControlPlane{}
	if err = r.Get(ctx, req.NamespacedName, eksCluster); err != nil {
		return ctrl.Result{}, microerror.Mask(err)
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
		BucketName:       key.BucketName(accountID, eksCluster.Name),
		ClusterName:      eksCluster.Name,
		ClusterNamespace: eksCluster.Namespace,
		ConfigName:       key.ConfigName(eksCluster.Name),
		Installation:     r.Installation,
		Region:           eksCluster.Spec.Region,
		// This is a hack to allow CAPI clusters to drop the 'release.giantswarm.io/version' label.
		ReleaseVersion: "20.0.0-alpha1",
		SecretName:     key.SecretName(eksCluster.Name),

		Logger:  logger,
		Cluster: eksCluster,
	})
	if err != nil {
		return reconcile.Result{}, microerror.Mask(err)
	}

	// Create IRSA service.
	irsaService := irsaEks.New(clusterScope, r.Client)

	if eksCluster.DeletionTimestamp != nil {
		finalizers := eksCluster.GetFinalizers()
		if !key.ContainsFinalizer(finalizers, key.FinalizerName) {
			return ctrl.Result{}, nil
		}
		logger.Info("Deleting IRSA resources for cluster")

		err := irsaService.Delete(ctx)
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}

		patchHelper, err := patch.NewHelper(eksCluster, r.Client)
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}
		controllerutil.RemoveFinalizer(eksCluster, key.FinalizerName)
		err = patchHelper.Patch(ctx, eksCluster)
		if err != nil {
			logger.Error(err, "failed to remove finalizer from AWSManagedControlPlane")
			return ctrl.Result{}, microerror.Mask(err)
		}
		logger.Info("successfully removed finalizer from AWSManagedControlPlane")

		r.sendEvent(eksCluster, v1.EventTypeNormal, "IRSA", "IRSA bootstrap deleted")
		return ctrl.Result{}, nil
	} else {
		created := false
		if !controllerutil.ContainsFinalizer(eksCluster, key.FinalizerName) {
			created = true

			patchHelper, err := patch.NewHelper(eksCluster, r.Client)
			if err != nil {
				return ctrl.Result{}, microerror.Mask(err)
			}
			controllerutil.AddFinalizer(eksCluster, key.FinalizerName)
			err = patchHelper.Patch(ctx, eksCluster)
			if err != nil {
				logger.Error(err, "failed to add finalizer on AWSManagedControlPlane")
				return ctrl.Result{}, microerror.Mask(err)
			}
			logger.Info("successfully added finalizer to AWSManagedControlPlane")
		}

		err := irsaService.Reconcile(ctx)
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}

		if created {
			r.sendEvent(eksCluster, v1.EventTypeNormal, "IRSA", "IRSA bootstrap created")
		}

		return ctrl.Result{
			Requeue: true,
		}, nil
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *EKSClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&eks.AWSManagedControlPlane{}).
		Complete(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.recorder = mgr.GetEventRecorderFor("irsa-eks-controller")
	return nil
}

func (r *EKSClusterReconciler) sendEvent(eksCluster *eks.AWSManagedControlPlane, eventtype, reason, message string) {
	r.recorder.Eventf(eksCluster, v1.EventTypeNormal, reason, message)
}
