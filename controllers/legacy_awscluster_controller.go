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

	infrastructurev1alpha3 "github.com/giantswarm/apiextensions/v3/pkg/apis/infrastructure/v1alpha3"
	"github.com/giantswarm/irsa-operator/pkg/aws/scope"
	"github.com/giantswarm/irsa-operator/pkg/irsa"
	"github.com/giantswarm/irsa-operator/pkg/key"
	"github.com/giantswarm/microerror"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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

	if _, ok := cluster.Annotations[key.IRSAAnnotation]; !ok {
		// resource does not contain IRSA annotation, nothing to do
		return ctrl.Result{}, nil
	}

	// fetch ARN from the cluster to assume role for creating dependencies
	credentialName := cluster.Spec.Provider.CredentialSecret.Name
	credentialNamespace := cluster.Spec.Provider.CredentialSecret.Namespace
	var credentialSecret = &v1.Secret{}
	if err = r.Get(ctx, types.NamespacedName{Namespace: credentialNamespace, Name: credentialName}, credentialSecret); err != nil {
		logger.Error(err, "failed to get credential secret")
		return ctrl.Result{}, microerror.Mask(err)
	}

	byte, ok := credentialSecret.Data["aws.awsoperator.arn"]
	if !ok {
		logger.Error(err, "Unable to extract ARN from secret")
		return ctrl.Result{}, microerror.Mask(fmt.Errorf("Unable to extract ARN from secret %s for cluster %s", credentialName, cluster.Name))

	}

	// convert secret data byte into string
	arn := string(byte)

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
		BucketName:       fmt.Sprintf("%s-%s-oidc-pod-identity", accountID, cluster.Name),
		ClusterName:      cluster.Name,
		ClusterNamespace: cluster.Namespace,
		Region:           cluster.Spec.Provider.Region,
		SecretName:       fmt.Sprintf("%s-service-account-v2", cluster.Name),

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

		controllerutil.RemoveFinalizer(cluster, key.FinalizerName)
		err = r.Update(ctx, cluster)
		if err != nil {
			logger.Error(err, "failed to remove finalizer on AWSCluster CR")
			return ctrl.Result{}, microerror.Mask(err)
		}
		return ctrl.Result{}, nil

	} else {
		err := irsaService.Reconcile(ctx)
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}

		controllerutil.AddFinalizer(cluster, key.FinalizerName)
		err = r.Update(ctx, cluster)
		if err != nil {
			logger.Error(err, "failed to add finalizer on AWSCluster CR")
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
