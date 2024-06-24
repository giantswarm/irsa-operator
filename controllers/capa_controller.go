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
	gocache "github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/tools/record"
	capa "sigs.k8s.io/cluster-api-provider-aws/v2/api/v1beta2"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/giantswarm/irsa-operator/pkg/aws/scope"
	irsaCapa "github.com/giantswarm/irsa-operator/pkg/irsa/capa"
	"github.com/giantswarm/irsa-operator/pkg/key"
)

const maxPatchRetries = 5

// CAPAClusterReconciler reconciles a CAPA AWSCluster object
type CAPAClusterReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	Installation string
	recorder     record.EventRecorder
	Cache        *gocache.Cache
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=awscluster,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=awscluster/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=awscluster/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *CAPAClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var err error
	logger := r.Log.WithValues("namespace", req.Namespace, "cluster", req.Name)

	logger.Info("Reconciling AWSCluster")

	awsCluster := &capa.AWSCluster{}
	if err := r.Get(ctx, req.NamespacedName, awsCluster); err != nil {
		return ctrl.Result{}, microerror.Mask(client.IgnoreNotFound(err))
	}

	if annotations.HasPaused(awsCluster) {
		logger.Info("AWSCluster is marked as paused, skipping")
		return ctrl.Result{}, nil
	}

	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, awsCluster.ObjectMeta)
	if err != nil {
		return reconcile.Result{}, microerror.Mask(err)
	}

	if awsCluster.DeletionTimestamp != nil || cluster.DeletionTimestamp != nil {
		finalizers := awsCluster.GetFinalizers()
		if !key.ContainsFinalizer(finalizers, key.FinalizerName) && !key.ContainsFinalizer(finalizers, key.FinalizerNameDeprecated) {
			return ctrl.Result{}, nil
		}
	}

	awsClusterRoleIdentity := &capa.AWSClusterRoleIdentity{}
	err = r.Get(ctx, types.NamespacedName{Name: awsCluster.Spec.IdentityRef.Name}, awsClusterRoleIdentity)
	if err != nil {
		return ctrl.Result{}, microerror.Mask(fmt.Errorf("failed to get AWSClusterRoleIdentity object %q: %w", awsCluster.Spec.IdentityRef.Name, err))
	}

	arn := awsClusterRoleIdentity.Spec.RoleArn

	// extract AccountID from ARN
	re := regexp.MustCompile(`[-]?\d[\d,]*[\.]?[\d{2}]*`)
	accountID := re.FindAllString(arn, 1)[0]

	if accountID == "" {
		logger.Error(err, "Unable to extract Account ID from ARN")
		return ctrl.Result{}, microerror.Mask(fmt.Errorf("unable to extract Account ID from ARN %s", string(arn)))
	}

	// Fetch config map created by cluster-apps-operator
	clusterValues := &v1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Namespace: awsCluster.Namespace, Name: fmt.Sprintf("%s-cluster-values", awsCluster.Name)}, clusterValues)
	if err != nil {
		return reconcile.Result{}, microerror.Mask(err)
	}

	baseDomain, err := getBaseDomain(clusterValues)
	if err != nil {
		return reconcile.Result{}, microerror.Mask(err)
	}

	// create the cluster scope.
	clusterScope, err := scope.NewClusterScope(scope.ClusterScopeParams{
		AccountID:        accountID,
		ARN:              arn,
		BaseDomain:       baseDomain,
		BucketName:       key.BucketName(accountID, awsCluster.Name),
		Cache:            r.Cache,
		ClusterName:      awsCluster.Name,
		ClusterNamespace: awsCluster.Namespace,
		ConfigName:       key.ConfigName(awsCluster.Name),
		Installation:     r.Installation,
		Region:           awsCluster.Spec.Region,
		// This is a hack to allow CAPI clusters to drop the 'release.giantswarm.io/version' label.
		ReleaseVersion: "20.0.0-alpha1",
		SecretName:     key.SecretName(awsCluster.Name),
		VPCMode:        awsCluster.Annotations["aws.giantswarm.io/vpc-mode"],

		Logger:  logger,
		Cluster: awsCluster,
	})
	if err != nil {
		return reconcile.Result{}, microerror.Mask(err)
	}

	// Create IRSA service.
	irsaService := irsaCapa.New(clusterScope, r.Client)

	if awsCluster.DeletionTimestamp != nil || cluster.DeletionTimestamp != nil {
		err := irsaService.Delete(ctx)
		if errors.Is(err, &irsaCapa.CloudfrontDistributionNotDisabledError{}) {
			// Distribution is not disabled yet, let's try again in 1 minute
			return ctrl.Result{RequeueAfter: time.Minute * 1}, nil
		}
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}

		patchHelperClusterValuesConfigMap, err := patch.NewHelper(clusterValues, r.Client)
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}
		controllerutil.RemoveFinalizer(clusterValues, key.FinalizerName)
		err = patchHelperClusterValuesConfigMap.Patch(ctx, clusterValues)
		if err != nil {
			logger.Error(err, "failed to remove finalizer from cluster values ConfigMap")
			return ctrl.Result{}, microerror.Mask(err)
		}
		logger.Info("successfully removed finalizer from cluster values ConfigMap")

		err = r.removeAWSClusterFinalizer(ctx, logger, awsCluster)
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}

		r.sendEvent(awsCluster, v1.EventTypeNormal, "IRSA", "IRSA bootstrap deleted")

		return ctrl.Result{}, nil
	} else {
		created := false
		// First add finalizer on cluster values ConfigMap since we need it to get the base domain (even on deletion)
		if !controllerutil.ContainsFinalizer(clusterValues, key.FinalizerName) {
			patchHelper, err := patch.NewHelper(clusterValues, r.Client)
			if err != nil {
				return ctrl.Result{}, microerror.Mask(err)
			}
			controllerutil.AddFinalizer(clusterValues, key.FinalizerName)
			err = patchHelper.Patch(ctx, clusterValues)
			if err != nil {
				logger.Error(err, "failed to add finalizer on cluster values ConfigMap")
				return ctrl.Result{}, microerror.Mask(err)
			}
			logger.Info("successfully added finalizer to cluster values ConfigMap")
		}

		if !controllerutil.ContainsFinalizer(awsCluster, key.FinalizerName) {
			created = true

			patchHelper, err := patch.NewHelper(awsCluster, r.Client)
			if err != nil {
				return ctrl.Result{}, microerror.Mask(err)
			}
			controllerutil.AddFinalizer(awsCluster, key.FinalizerName)
			err = patchHelper.Patch(ctx, awsCluster)
			if err != nil {
				logger.Error(err, "failed to add finalizer on AWSCluster")
				return ctrl.Result{}, microerror.Mask(err)
			}
			logger.Info("successfully added finalizer to AWSCluster")
		}

		// Re-run regularly to ensure OIDC certificate thumbprints are up to date (see `EnsureOIDCProviders`)
		requeueAfter := time.Minute * 5

		err := irsaService.Reconcile(ctx, &requeueAfter)
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}

		if created {
			r.sendEvent(awsCluster, v1.EventTypeNormal, "IRSA", "IRSA bootstrap created")
		}

		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
}

func (r *CAPAClusterReconciler) removeAWSClusterFinalizer(ctx context.Context, logger logr.Logger, cluster *capa.AWSCluster) error {
	for i := 1; i <= maxPatchRetries; i++ {
		patchedCluster := cluster.DeepCopy()
		controllerutil.RemoveFinalizer(patchedCluster, key.FinalizerNameDeprecated)
		controllerutil.RemoveFinalizer(patchedCluster, key.FinalizerName)
		err := r.Patch(ctx, patchedCluster, client.MergeFrom(cluster))

		// If another controller has removed its finalizer while we're
		// reconciling this will fail with "Forbidden: no new finalizers can be
		// added if the object is being deleted". The actual response code is
		// 422 Unprocessable entity, which maps to StatusReasonInvalid in the
		// k8serrors package. We have to get the cluster again with the now
		// removed finalizer(s) and try again.
		if k8serrors.IsInvalid(err) && i < maxPatchRetries {
			logger.Info("patching AWSCluster failed, trying again: %s", err.Error())
			if err := r.Get(ctx, client.ObjectKeyFromObject(cluster), cluster); err != nil {
				return microerror.Mask(err)
			}
			continue
		}
		if err != nil {
			logger.Error(err, "failed to remove finalizers from AWSCluster")
			return microerror.Mask(err)
		}
	}
	logger.Info("successfully removed finalizer from AWSCluster")
	return nil
}

func getBaseDomain(clusterValuesConfigMap *v1.ConfigMap) (string, error) {
	jsonStr := clusterValuesConfigMap.Data["values"]
	if jsonStr == "" {
		return "", microerror.Mask(clusterValuesConfigMapNotFound)
	}

	type clusterValues struct {
		BaseDomain string `yaml:"baseDomain"`
	}

	cv := clusterValues{}

	err := yaml.Unmarshal([]byte(jsonStr), &cv)
	if err != nil {
		return "", err
	}

	baseDomain := cv.BaseDomain
	if baseDomain == "" {
		return "", microerror.Mask(baseDomainNotFound)
	}

	return baseDomain, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CAPAClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&capa.AWSCluster{}).
		Complete(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.recorder = mgr.GetEventRecorderFor("irsa-capa-controller")
	return nil
}

func (r *CAPAClusterReconciler) sendEvent(cluster *capa.AWSCluster, eventtype, reason, message string) {
	r.recorder.Eventf(cluster, v1.EventTypeNormal, reason, message)
}
