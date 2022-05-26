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
	"os"
	"time"

	buildv1 "github.com/openshift/api/build/v1"
	imagev1 "github.com/openshift/api/image/v1"
	secv1 "github.com/openshift/api/security/v1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"

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
)

const (
	minDelaySR   = 100 * time.Millisecond
	maxDelaySR   = 3 * time.Second
	SRgvk        = "SpecialResource"
	SROwnedLabel = "specialresource.openshift.io/owned"
)

// SpecialResourceReconciler reconciles a SpecialResource object
type SpecialResourceReconciler struct {
	Scheme *k8sruntime.Scheme

	Metrics       metrics.Metrics
	Cluster       cluster.Cluster
	ClusterInfo   upgrade.ClusterInfo
	ResourceAPI   resource.ResourceAPI
	Filter        filter.Filter
	Finalizer     finalizers.SpecialResourceFinalizer
	Helmer        helmer.Helmer
	Assets        assets.Assets
	PollActions   poll.PollActions
	StatusUpdater state.StatusUpdater
	Storage       storage.Storage
	KernelData    kernel.KernelData
	ProxyAPI      proxy.ProxyAPI
	RuntimeAPI    runtime.RuntimeAPI
	KubeClient    clients.ClientsInterface
}

// Reconcile Reconiliation entry point
func (r *SpecialResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconciling", "mode", r.Filter.GetMode())

	sr, srs, err := r.getSpecialResources(ctx, req)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get SpecialResources: %w", err)
	} else if sr == nil {
		log.Info("SpecialResource not found - probably deleted. Not reconciling.")
		return ctrl.Result{}, nil
	}

	r.Metrics.SetSpecialResourcesCreated(len(srs.Items))

	wi := &WorkItem{
		SpecialResource: sr,
		AllSRs:          srs,
	}

	// Reconcile all specialresources
	return r.SpecialResourcesReconcile(ctx, wi)
}

func (r *SpecialResourceReconciler) getSpecialResources(ctx context.Context, req ctrl.Request) (*srov1beta1.SpecialResource, *srov1beta1.SpecialResourceList, error) {
	specialresources := &srov1beta1.SpecialResourceList{}

	err := r.KubeClient.List(ctx, specialresources)
	if err != nil {
		return nil, nil, fmt.Errorf("couldn't list SpecialResources: %w", err)
	}

	var idx int
	var found bool
	if idx, found = FindSR(specialresources.Items, req.Name, "Name"); !found {
		// If we do not find the specialresource it might be deleted,
		// if it is a depdendency of another specialresource assign the
		// parent specialresource for processing.
		obj := types.NamespacedName{
			Namespace: os.Getenv("OPERATOR_NAMESPACE"),
			Name:      "special-resource-dependencies",
		}
		parent, err := r.Storage.CheckConfigMapEntry(ctx, req.Name, obj)
		if err != nil {
			return nil, nil, fmt.Errorf("couldn't check configmap entry: %w", err)
		}

		idx, found = FindSR(specialresources.Items, parent, "Name")
		if !found {
			return nil, nil, nil
		}
	}

	return &specialresources.Items[idx], specialresources, nil
}

// SetupWithManager main initalization for manager
func (r *SpecialResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("specialresource").
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
			RateLimiter:             workqueue.NewItemExponentialFailureRateLimiter(minDelaySR, maxDelaySR),
		}).
		WithEventFilter(r.Filter.GetPredicates()).
		Complete(r)
}
