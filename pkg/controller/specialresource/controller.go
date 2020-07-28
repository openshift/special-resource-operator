package specialresource

import (
	"context"
	"fmt"

	monitoringV1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	srov1alpha1 "github.com/openshift-psap/special-resource-operator/pkg/apis/sro/v1alpha1"
	buildV1 "github.com/openshift/api/build/v1"
	imageV1 "github.com/openshift/api/image/v1"
	routev1 "github.com/openshift/api/route/v1"
	secv1 "github.com/openshift/api/security/v1"
	errs "github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("specialresource")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new SpecialResource Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileSpecialResource{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("specialresource", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource SpecialResource
	err = c.Watch(&source.Kind{Type: &srov1alpha1.SpecialResource{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner SpecialResource
	if err = c.Watch(&source.Kind{Type: &appsv1.DaemonSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &srov1alpha1.SpecialResource{},
	}); err != nil {
		return err
	}

	if err = c.Watch(&source.Kind{Type: &corev1.ServiceAccount{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &srov1alpha1.SpecialResource{},
	}); err != nil {
		return err
	}

	if err = c.Watch(&source.Kind{Type: &secv1.SecurityContextConstraints{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &srov1alpha1.SpecialResource{},
	}); err != nil {
		return err
	}

	if err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &srov1alpha1.SpecialResource{},
	}); err != nil {
		return err
	}

	if err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &srov1alpha1.SpecialResource{},
	}); err != nil {
		return err
	}

	if err = c.Watch(&source.Kind{Type: &rbacv1.Role{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &srov1alpha1.SpecialResource{},
	}); err != nil {
		return err
	}

	if err = c.Watch(&source.Kind{Type: &rbacv1.RoleBinding{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &srov1alpha1.SpecialResource{},
	}); err != nil {
		return err
	}

	if err = c.Watch(&source.Kind{Type: &routev1.Route{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &srov1alpha1.SpecialResource{},
	}); err != nil {
		return err
	}

	if err = c.Watch(&source.Kind{Type: &buildV1.BuildConfig{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &srov1alpha1.SpecialResource{},
	}); err != nil {
		return err
	}

	if err = c.Watch(&source.Kind{Type: &imageV1.ImageStream{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &srov1alpha1.SpecialResource{},
	}); err != nil {
		return err
	}

	if err = c.Watch(&source.Kind{Type: &monitoringV1.PrometheusRule{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &srov1alpha1.SpecialResource{},
	}); err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileSpecialResource implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileSpecialResource{}

// ReconcileSpecialResource reconciles a SpecialResource object
type ReconcileSpecialResource struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client          client.Client
	scheme          *runtime.Scheme
	specialresource srov1alpha1.SpecialResource
}

// Reconcile reads that state of the cluster for a SpecialResource object and makes changes based on the state read
// and what is in the SpecialResource.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileSpecialResource) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Namespace", request.Namespace, "Name", request.Name)
	reqLogger.Info("Reconciling SpecialResource")

	// Fetch the SpecialResource instance
	specialresources := srov1alpha1.SpecialResourceList{}
	opts := &client.ListOptions{}
	opts.InNamespace(request.Namespace)

	err := r.client.List(context.TODO(), opts, &specialresources)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	for _, specialresource := range specialresources.Items {

		log.Info("Reconciling", "SpecialResource", specialresource.Name)
		log.Info("SpecialResurce", "DependsOn", specialresource.Spec.DependsOn.Name)

		// Only one level dependency support for now
		for _, dependency := range specialresource.Spec.DependsOn.Name {
			r.specialresource = getSpecialResourceByName(dependency, &specialresources)
			if err := ReconcileHardwareConfigurations(r); err != nil {
				// We do not want a stacktrace here, errs.Wrap already created
				// breadcrumb of errors to follow. Just sprintf with %v rather than %+v
				log.Info("Could not reconcile hardware configurations", "error", fmt.Sprintf("%v", err))
				return reconcile.Result{}, errs.New("Reconciling failed")
			}
		}

		r.specialresource = specialresource
		if err := ReconcileHardwareConfigurations(r); err != nil {
			// We do not want a stacktrace here, errs.Wrap already created
			// breadcrumb of errors to follow. Just sprintf with %v rather than %+v
			log.Info("Could not reconcile hardware configurations", "error", fmt.Sprintf("%v", err))
			return reconcile.Result{}, errs.New("Reconciling failed")
		}

	}

	return reconcile.Result{}, nil
}

func getSpecialResourceByName(name string, list *srov1alpha1.SpecialResourceList) srov1alpha1.SpecialResource {
	for _, specialresource := range list.Items {
		if specialresource.Name == name {
			return specialresource
		}
	}
	return srov1alpha1.SpecialResource{}
}
