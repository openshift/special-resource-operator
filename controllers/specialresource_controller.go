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
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	srov1beta1 "github.com/openshift-psap/special-resource-operator/api/v1beta1"
)

var (
	log logr.Logger
)

// SpecialResourceReconciler reconciles a SpecialResource object
type SpecialResourceReconciler struct {
	client.Client
	Log             logr.Logger
	Scheme          *runtime.Scheme
	specialresource srov1beta1.SpecialResource
	parent          srov1beta1.SpecialResource
	dependency      srov1beta1.SpecialResourceDependency
}

func (r *SpecialResourceReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	return ReconcilerSpecialResources(r, req)
}

func (r *SpecialResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&srov1beta1.SpecialResource{}).
		Owns(&v1.Pod{}).
		Owns(&appsv1.DaemonSet{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		}).
		Complete(r)
}
