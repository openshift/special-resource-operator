/*


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

	"github.com/go-logr/logr"
	buildv1 "github.com/openshift/api/build/v1"
	imagev1 "github.com/openshift/api/image/v1"
	secv1 "github.com/openshift/api/security/v1"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	srov1beta1 "github.com/openshift/special-resource-operator/api/v1beta1"
	"github.com/openshift/special-resource-operator/internal/controllers/finalizers"
	"github.com/openshift/special-resource-operator/internal/controllers/state"
	"github.com/openshift/special-resource-operator/pkg/assets"
	"github.com/openshift/special-resource-operator/pkg/clients"
	"github.com/openshift/special-resource-operator/pkg/cluster"
	"github.com/openshift/special-resource-operator/pkg/filter"
	"github.com/openshift/special-resource-operator/pkg/helmer"
	"github.com/openshift/special-resource-operator/pkg/kernel"
	"github.com/openshift/special-resource-operator/pkg/metrics"
	"github.com/openshift/special-resource-operator/pkg/poll"
	"github.com/openshift/special-resource-operator/pkg/proxy"
	"github.com/openshift/special-resource-operator/pkg/resource"
	"github.com/openshift/special-resource-operator/pkg/runtime"
	"github.com/openshift/special-resource-operator/pkg/storage"
	"github.com/openshift/special-resource-operator/pkg/upgrade"
	"github.com/openshift/special-resource-operator/pkg/utils"
)

// SpecialResourceReconciler reconciles a SpecialResource object
type SpecialResourceReconciler struct {
	Log    logr.Logger
	Scheme *k8sruntime.Scheme

	Metrics                metrics.Metrics
	Cluster                cluster.Cluster
	ClusterInfo            upgrade.ClusterInfo
	ClusterOperatorManager state.ClusterOperatorManager
	Creator                resource.Creator
	Filter                 filter.Filter
	Finalizer              finalizers.SpecialResourceFinalizer
	Helmer                 helmer.Helmer
	Assets                 assets.Assets
	PollActions            poll.PollActions
	StatusUpdater          state.StatusUpdater
	Storage                storage.Storage
	KernelData             kernel.KernelData
	ProxyAPI               proxy.ProxyAPI
	RuntimeAPI             runtime.RuntimeAPI
	KubeClient             clients.ClientsInterface
}

// Reconcile Reconiliation entry point
func (r *SpecialResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	var res reconcile.Result

	log := r.Log.WithName(utils.Print(req.Name, utils.Purple))
	log.Info("Reconciling", "mode", r.Filter.GetMode())

	log.Info("TODO: preflight checks")

	sr, srs, err := r.getSpecialResources(ctx, req)
	if err != nil {
		log.Error(err, "failed to get SpecialResources")
		return ctrl.Result{}, err
	} else if sr == nil {
		log.Info("SpecialResource not found - probably deleted. Not reconciling.")
		return ctrl.Result{}, nil
	}

	r.Metrics.SetSpecialResourcesCreated(len(srs.Items))

	wi := &WorkItem{
		SpecialResource: sr,
		AllSRs:          srs,
		Log:             log,
	}

	conds := utils.NotAvailableProgressingNotDegraded(
		"Reconciling "+req.Name,
		"Reconciling "+req.Name,
		utils.DegradedDefaultMsg,
	)

	// A resource is being reconciled set status to not available and only
	// if the reconcilation succeeds we're updating the conditions
	if err = r.ClusterOperatorManager.Refresh(ctx, conds); err != nil {
		res.Requeue = true
		return res, errors.Wrap(err, "Failed to update ClusterOperator's status")
	}

	// Reconcile all specialresources
	if res, err = r.SpecialResourcesReconcile(ctx, wi); err == nil && !res.Requeue {
		conds = utils.AvailableNotProgressingNotDegraded()
	} else {
		return res, errors.Wrap(err, "Failed to reconcile SpecialResource")
	}

	// Only if we're successfull we're going to update the status to
	// Available otherwise return the reconcile error
	if err = r.ClusterOperatorManager.Refresh(ctx, conds); err != nil {
		res.Requeue = true
		log.Error(err, "Failed to update ClusterOperator's status")
		return res, nil
	}

	log.Info("Reconciliation successful")
	return reconcile.Result{}, nil
}

// SetupWithManager main initalization for manager
func (r *SpecialResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	log := r.Log.WithName(utils.Print("setup", utils.Brown))

	platform, err := r.KubeClient.GetPlatform()
	if err != nil {
		return err
	}

	if platform == "OCP" {
		return ctrl.NewControllerManagedBy(mgr).
			For(&srov1beta1.SpecialResource{}).
			Owns(&v1.Pod{}).
			Owns(&appsv1.DaemonSet{}).
			Owns(&appsv1.Deployment{}).
			Owns(&storagev1.CSIDriver{}).
			Owns(&imagev1.ImageStream{}).
			Owns(&buildv1.BuildConfig{}).
			Owns(&v1.ConfigMap{}).
			Owns(&v1.ServiceAccount{}).
			Owns(&rbacv1.Role{}).
			Owns(&rbacv1.RoleBinding{}).
			Owns(&rbacv1.ClusterRole{}).
			Owns(&rbacv1.ClusterRoleBinding{}).
			Owns(&secv1.SecurityContextConstraints{}).
			Owns(&v1.Secret{}).
			WithOptions(controller.Options{
				MaxConcurrentReconciles: 1,
			}).
			WithEventFilter(r.Filter.GetPredicates()).
			Complete(r)
	} else {
		log.Info("Warning: assuming vanilla K8s. Manager will own a limited set of resources.")
		return ctrl.NewControllerManagedBy(mgr).
			For(&srov1beta1.SpecialResource{}).
			Owns(&v1.Pod{}).
			Owns(&appsv1.DaemonSet{}).
			Owns(&appsv1.Deployment{}).
			Owns(&storagev1.CSIDriver{}).
			Owns(&v1.ConfigMap{}).
			Owns(&v1.ServiceAccount{}).
			Owns(&rbacv1.Role{}).
			Owns(&rbacv1.RoleBinding{}).
			Owns(&rbacv1.ClusterRole{}).
			Owns(&rbacv1.ClusterRoleBinding{}).
			Owns(&v1.Secret{}).
			WithOptions(controller.Options{
				MaxConcurrentReconciles: 1,
			}).
			WithEventFilter(r.Filter.GetPredicates()).
			Complete(r)
	}
}
