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
	"github.com/giantswarm/irsa-operator/pkg/aws/services/iam"
	"github.com/giantswarm/irsa-operator/pkg/aws/services/s3"
	"github.com/giantswarm/irsa-operator/pkg/files"
	"github.com/giantswarm/irsa-operator/pkg/key"
	"github.com/giantswarm/microerror"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	// Create the cluster scope.
	clusterScope, err := scope.NewClusterScope(scope.ClusterScopeParams{
		AccountID:  accountID,
		ARN:        arn,
		Region:     cluster.Spec.Provider.Region,
		BucketName: fmt.Sprintf("%s-%s-oidc-pod-identity", accountID, cluster.Name),

		Logger:     logger,
		AWSCluster: cluster,
	})
	if err != nil {
		return reconcile.Result{}, microerror.Mask(err)
	}

	iamService := iam.NewService(clusterScope)
	s3Service := s3.NewService(clusterScope)

	if cluster.DeletionTimestamp != nil {
		err := s3Service.DeleteFiles(clusterScope.BucketName())
		if err != nil {
			logger.Error(err, "failed to delete files in S3 bucket")
			return ctrl.Result{}, microerror.Mask(err)
		}
		err = s3Service.DeleteBucket(clusterScope.BucketName())
		if err != nil {
			logger.Error(err, "failed to delete S3 bucket")
			return ctrl.Result{}, microerror.Mask(err)
		}
		err = iamService.DeleteOIDCProvider(clusterScope.AccountID(), clusterScope.BucketName(), clusterScope.Region())
		if err != nil {
			logger.Error(err, "failed to delete OIDC")
			return ctrl.Result{}, microerror.Mask(err)
		}

		controllerutil.RemoveFinalizer(cluster, key.FinalizerName)
		err = r.Update(ctx, cluster)
		if err != nil {
			logger.Error(err, "failed to remove finalizer on AWSCluster CR")
			return ctrl.Result{}, microerror.Mask(err)
		}
		// resource was cleaned up, we dont need to reconcile again
		return ctrl.Result{}, nil

	} else {
		oidcSecretName := fmt.Sprintf("%s-service-account-v2", cluster.Name)
		if err := r.Get(ctx, types.NamespacedName{Namespace: cluster.Namespace, Name: oidcSecretName}, &v1.Secret{}); err != nil {
			logger.Error(err, "OIDC Service Account Secret does not exist")
			//TODO check creation only once by checking secret present
			err := files.Generate(clusterScope.BucketName(), clusterScope.Region())
			if err != nil {
				logger.Error(err, "failed to generate files for cluster")
				return ctrl.Result{}, microerror.Mask(err)
			}

			privateKey, err := files.ReadFile(clusterScope.BucketName(), "signer.key")
			if err != nil {
				logger.Error(err, "failed to read private key file for cluster")
				return ctrl.Result{}, microerror.Mask(err)

			}
			publicKey, err := files.ReadFile(clusterScope.BucketName(), "signer.pub")
			if err != nil {
				logger.Error(err, "failed to read public key file for cluster")
				return ctrl.Result{}, microerror.Mask(err)

			}

			oidcSecret := &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      oidcSecretName,
					Namespace: cluster.Namespace,
				},
				Data: map[string][]byte{
					"service-account-v2.key": privateKey,
					"service-account-v2.pub": publicKey,
				},
				Type: v1.SecretTypeOpaque,
			}
			if err := r.Create(ctx, oidcSecret); err != nil {

			}
		}
		err = s3Service.CreateBucket(clusterScope.BucketName())
		if err != nil {
			logger.Error(err, "failed to create bucket")
			return ctrl.Result{}, microerror.Mask(err)
		}
		err = s3Service.UploadFiles(clusterScope.BucketName())
		if err != nil {
			logger.Error(err, "failed to upload files")
			return ctrl.Result{}, microerror.Mask(err)
		}
		err = iamService.CreateOIDCProvider(clusterScope.BucketName(), clusterScope.Region())
		if err != nil {
			logger.Error(err, "failed to create OIDC provider")
			return ctrl.Result{}, microerror.Mask(err)
		}

		controllerutil.AddFinalizer(cluster, key.FinalizerName)
		err = r.Update(ctx, cluster)
		if err != nil {
			logger.Error(err, "failed to add finalizer on AWSCluster CR")
			return ctrl.Result{}, microerror.Mask(err)
		}
		return ctrl.Result{}, nil
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
