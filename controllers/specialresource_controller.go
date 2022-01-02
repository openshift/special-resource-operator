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
	"fmt"

	"github.com/go-logr/logr"
	srov1beta1 "github.com/openshift-psap/special-resource-operator/api/v1beta1"
	"github.com/openshift-psap/special-resource-operator/pkg/assets"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/cluster"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"github.com/openshift-psap/special-resource-operator/pkg/conditions"
	"github.com/openshift-psap/special-resource-operator/pkg/filter"
	"github.com/openshift-psap/special-resource-operator/pkg/helmer"
	"github.com/openshift-psap/special-resource-operator/pkg/kernel"
	"github.com/openshift-psap/special-resource-operator/pkg/metrics"
	"github.com/openshift-psap/special-resource-operator/pkg/poll"
	"github.com/openshift-psap/special-resource-operator/pkg/proxy"
	"github.com/openshift-psap/special-resource-operator/pkg/resource"
	"github.com/openshift-psap/special-resource-operator/pkg/storage"
	"github.com/openshift-psap/special-resource-operator/pkg/upgrade"
	buildv1 "github.com/openshift/api/build/v1"
	secv1 "github.com/openshift/api/security/v1"

	imagev1 "github.com/openshift/api/image/v1"
	"github.com/pkg/errors"

	configv1 "github.com/openshift/api/config/v1"

	"helm.sh/helm/v3/pkg/chart"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	log logr.Logger
)

// SpecialResourceReconciler reconciles a SpecialResource object
type SpecialResourceReconciler struct {
	Log    logr.Logger
	Scheme *runtime.Scheme

	Metrics     metrics.Metrics
	Cluster     cluster.Cluster
	ClusterInfo upgrade.ClusterInfo
	Creator     resource.Creator
	Filter      filter.Filter
	Helmer      helmer.Helmer
	Assets      assets.Assets
	PollActions poll.PollActions
	Storage     storage.Storage
	KernelData  kernel.KernelData
	ProxyAPI    proxy.ProxyAPI

	specialresource srov1beta1.SpecialResource
	parent          srov1beta1.SpecialResource
	chart           chart.Chart
	values          unstructured.Unstructured
	dependency      srov1beta1.SpecialResourceDependency
	clusterOperator configv1.ClusterOperator
}

// Reconcile Reconiliation entry point
func (r *SpecialResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	var err error
	var res reconcile.Result

	log = r.Log.WithName(color.Print("preamble", color.Brown))
	log.Info("Controller Request", "Name", req.Name, "Namespace", req.Namespace)

	conds := conditions.NotAvailableProgressingNotDegraded(
		"Reconciling "+req.Name,
		"Reconciling "+req.Name,
		conditions.DegradedDefaultMsg,
	)

	// Do some preflight checks and get the cluster upgrade info
	if res, err = SpecialResourceUpgrade(r, req); err != nil {
		return res, errors.Wrap(err, "RECONCILE ERROR: Cannot upgrade special resource")
	}
	// A resource is being reconciled set status to not available and only
	// if the reconcilation succeeds we're updating the conditions
	if res, err = SpecialResourcesStatus(r, req, conds); err != nil {
		return res, errors.Wrap(err, "RECONCILE ERROR: Cannot update special resource status")
	}
	// Reconcile all specialresources
	if res, err = SpecialResourcesReconcile(r, req); err == nil && !res.Requeue {
		conds = conditions.AvailableNotProgressingNotDegraded()
	} else {
		return res, errors.Wrap(err, "RECONCILE ERROR: Cannot reconcile special resource")
	}

	// Only if we're successfull we're going to update the status to
	// Available otherwise return the reconcile error
	if res, err = SpecialResourcesStatus(r, req, conds); err != nil {
		log.Info("RECONCILE ERROR: Cannot update special resource status", "error", fmt.Sprintf("%v", err))
		return res, nil
	}
	log.Info("RECONCILE SUCCESS: Reconcile")
	return reconcile.Result{}, nil
}

// SetupWithManager main initalization for manager
func (r *SpecialResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	log = r.Log.WithName(color.Print("setup", color.Brown))

	platform, err := clients.Interface.GetPlatform()
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
