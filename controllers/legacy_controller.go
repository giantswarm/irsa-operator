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

	"github.com/blang/semver"
	infrastructurev1alpha3 "github.com/giantswarm/apiextensions/v6/pkg/apis/infrastructure/v1alpha3"
	"github.com/giantswarm/microerror"
	"github.com/go-logr/logr"
	gocache "github.com/patrickmn/go-cache"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/giantswarm/irsa-operator/pkg/aws/scope"
	irsaLegacy "github.com/giantswarm/irsa-operator/pkg/irsa/legacy"
	"github.com/giantswarm/irsa-operator/pkg/key"
)

// LegacyClusterReconciler reconciles a Giant Swarm AWSCluster object
type LegacyClusterReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	Installation string
	recorder     record.EventRecorder
	Cache        *gocache.Cache
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

	awsCluster := &infrastructurev1alpha3.AWSCluster{}
	if err := r.Get(ctx, req.NamespacedName, awsCluster); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Cluster no longer exists")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, microerror.Mask(err)
	}

	releaseVersion, err := semver.New(key.Release(awsCluster))
	if err != nil {
		logger.Error(err, "Unable to extract release from AWSCluster CR")
		return ctrl.Result{}, microerror.Mask(err)
	}

	if !key.IsV19Release(releaseVersion) {
		if _, ok := awsCluster.Annotations[key.IRSAAnnotation]; !ok {
			logger.Info(fmt.Sprintf(
				"AWSCluster CR do not have required annotation '%s' or release version is not v19.0.0 or higher, ignoring CR",
				key.IRSAAnnotation))
			return ctrl.Result{
				Requeue:      true,
				RequeueAfter: time.Minute * 5,
			}, nil
		}
	}

	// check if cluster needs to be migrated
	cm := &v1.ConfigMap{}
	var migration bool
	if err := r.Get(ctx, types.NamespacedName{Name: "irsa-migration", Namespace: "giantswarm"}, cm); err != nil {
		if errors.IsNotFound(err) {
			// no migration needed
		} else if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}
	}
	if _, ok := cm.Data[awsCluster.Name]; ok {
		migration = true
	}

	// fetch ARN from the cluster to assume role for creating dependencies
	credentialName := awsCluster.Spec.Provider.CredentialSecret.Name
	credentialNamespace := awsCluster.Spec.Provider.CredentialSecret.Namespace
	var credentialSecret = &v1.Secret{}
	if err = r.Get(ctx, types.NamespacedName{Namespace: credentialNamespace, Name: credentialName}, credentialSecret); err != nil {
		logger.Error(err, "failed to get credential secret")
		return ctrl.Result{}, microerror.Mask(err)
	}

	secretByte, ok := credentialSecret.Data["aws.awsoperator.arn"]
	if !ok {
		logger.Error(err, "unable to extract ARN from secret")
		return ctrl.Result{}, microerror.Mask(fmt.Errorf("unable to extract ARN from secret %s for cluster %s", credentialName, awsCluster.Name))

	}

	// convert secret data secretByte into string
	arn := string(secretByte)

	// extract AccountID from ARN
	re := regexp.MustCompile(`[-]?\d[\d,]*[\.]?[\d{2}]*`)
	accountID := re.FindAllString(arn, 1)[0]

	if accountID == "" {
		logger.Error(err, "unable to extract Account ID from ARN")
		return ctrl.Result{}, microerror.Mask(fmt.Errorf("unable to extract Account ID from ARN %s", string(arn)))
	}

	// Check if Cloudfront alias should be used before v19.0.0
	_, preCloudfrontAlias := awsCluster.Annotations[key.IRSAPreCloudfrontAliasAnnotation]

	keepCloudFrontOIDCProvider := awsCluster.Annotations[key.KeepCloudFrontOIDCProviderAnnotation]
	if keepCloudFrontOIDCProvider == "" {
		keepCloudFrontOIDCProvider = "true"
	}
	if keepCloudFrontOIDCProvider != "true" && keepCloudFrontOIDCProvider != "false" {
		return ctrl.Result{}, microerror.Mask(fmt.Errorf("invalid value %q in annotation %q, only `\"true\"` and `\"false\"` are allowed", keepCloudFrontOIDCProvider, key.KeepCloudFrontOIDCProviderAnnotation))
	}

	// create the cluster scope.
	clusterScope, err := scope.NewClusterScope(scope.ClusterScopeParams{
		AccountID:                  accountID,
		ARN:                        arn,
		BucketName:                 key.BucketName(accountID, awsCluster.Name),
		Cache:                      r.Cache,
		ClusterName:                awsCluster.Name,
		ClusterNamespace:           awsCluster.Namespace,
		ConfigName:                 key.ConfigName(awsCluster.Name),
		Installation:               r.Installation,
		KeepCloudFrontOIDCProvider: keepCloudFrontOIDCProvider != "false",
		Migration:                  migration,
		PreCloudfrontAlias:         preCloudfrontAlias,
		Region:                     awsCluster.Spec.Provider.Region,
		ReleaseVersion:             key.Release(awsCluster),
		SecretName:                 key.SecretName(awsCluster.Name),

		Logger:  logger,
		Cluster: awsCluster,
	})
	if err != nil {
		return reconcile.Result{}, microerror.Mask(err)
	}

	// Create IRSA service for Vintage.
	irsaService := irsaLegacy.New(clusterScope, r.Client)

	if !awsCluster.DeletionTimestamp.IsZero() {
		finalizers := awsCluster.GetFinalizers()
		if !key.ContainsFinalizer(finalizers, key.FinalizerName) && !key.ContainsFinalizer(finalizers, key.FinalizerNameDeprecated) {
			return ctrl.Result{}, nil
		}

		err := irsaService.Delete(ctx, awsCluster)
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}

		patchHelper, err := patch.NewHelper(awsCluster, r.Client)
		if err != nil {
			return ctrl.Result{}, err
		}
		controllerutil.RemoveFinalizer(awsCluster, key.FinalizerNameDeprecated)
		controllerutil.RemoveFinalizer(awsCluster, key.FinalizerName)
		err = patchHelper.Patch(ctx, awsCluster)
		if err != nil {
			logger.Error(err, "failed to remove finalizer from AWSCluster")
			return ctrl.Result{}, err
		}
		logger.Info("successfully removed finalizer from AWSCluster")

		r.sendEvent(awsCluster, v1.EventTypeNormal, "IRSA", "IRSA bootstrap deleted")

		return ctrl.Result{}, nil

	} else {
		created := false
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

		err := irsaService.Reconcile(ctx)
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}

		if created {
			r.sendEvent(awsCluster, v1.EventTypeNormal, "IRSA", "IRSA bootstrap created")
		}

		// Re-run regularly to ensure OIDC certificate thumbprints are up to date (see `EnsureOIDCProviders`)
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: time.Minute * 5,
		}, nil
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *LegacyClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.recorder = mgr.GetEventRecorderFor("irsa-legacy-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha3.AWSCluster{}).
		Complete(r)
}

func (r *LegacyClusterReconciler) sendEvent(cluster *infrastructurev1alpha3.AWSCluster, eventtype, reason, message string) {
	r.recorder.Event(cluster, eventtype, reason, message)
}
