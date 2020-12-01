package controllers

import (
	"context"
	"fmt"

	srov1beta1 "github.com/openshift-psap/special-resource-operator/api/v1beta1"
	errs "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// +kubebuilder:rbac:groups=sro.openshift.io,resources=specialresources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sro.openshift.io,resources=specialresources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=sro.openshift.io,resources=specialresources/finalizers,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods/log,verbs=get
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=config.openshift.io,resources=clusterversions,verbs=get
// +kubebuilder:rbac:groups=config.openshift.io,resources=proxies,verbs=get;list
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,verbs=use;get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=image.openshift.io,resources=imagestreams,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=image.openshift.io,resources=imagestreams/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=image.openshift.io,resources=imagestreams/layers,verbs=get
// +kubebuilder:rbac:groups=core,resources=imagestreams/layers,verbs=get
// +kubebuilder:rbac:groups=build.openshift.io,resources=buildconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=build.openshift.io,resources=builds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=list;watch;create;update;patch
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;update;
// +kubebuilder:rbac:groups=core,resources=persistentvolumes,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=storage.k8s.io,resources=csinodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=watch
// +kubebuilder:rbac:groups=storage.k8s.io,resources=csidrivers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=endpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=prometheusrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// ReconcilerSpecialResources Takes care of all specialresources in the cluster
func ReconcilerSpecialResources(r *SpecialResourceReconciler, req ctrl.Request) (ctrl.Result, error) {

	r.Log.Info("Reconciling SpecialResource(s) in all Namespaces")

	specialresources := &srov1beta1.SpecialResourceList{}
	opts := []client.ListOption{}
	err := r.List(context.TODO(), specialresources, opts...)
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

	for _, r.parent = range specialresources.Items {

		//log = r.Log.WithValues("specialresource", r.parent.Name)
		log = r.Log.WithName(prettyPrint(r.parent.Name, Green))
		log.Info("Resolving Dependencies")

		// Only one level dependency support for now
		for _, r.dependency = range r.parent.Spec.DependsOn {

			//log = r.Log.WithValues("specialresource", r.dependency.Name)
			log = r.Log.WithName(prettyPrint(r.dependency.Name, Purple))
			log.Info("Getting Dependency")

			// Assign the specialresource to the reconciler object
			if r.specialresource, err = getDependencyFrom(specialresources, r.dependency.Name); err != nil {
				log.Info("Could not get SpecialResource dependency", "error", fmt.Sprintf("%v", err))
				if r.specialresource, err = createSpecialResourceFrom(r, r.dependency.Name); err != nil {
					//return reconcile.Result{}, errs.New("Dependency creation failed")
					log.Info("Dependency creation failed", "error", fmt.Sprintf("%v", err))
					return reconcile.Result{Requeue: true}, nil
				}
				// We need to fetch the newly created SpecialResources, reconciling
				return reconcile.Result{}, nil
			}

			log.Info("Reconciling Dependency")
			if err := ReconcileHardwareConfigurations(r); err != nil {
				// We do not want a stacktrace here, errs.Wrap already created
				// breadcrumb of errors to follow. Just sprintf with %v rather than %+v
				log.Info("Could not reconcile hardware configurations", "error", fmt.Sprintf("%v", err))
				//return reconcile.Result{}, errs.New("Reconciling failed")
				return reconcile.Result{Requeue: true}, nil
			}
		}

		//log = r.Log.WithValues("specialresource", r.parent.Name)
		log = r.Log.WithName(prettyPrint(r.parent.Name, Green))
		log.Info("Reconciling")

		r.specialresource = r.parent
		if err := ReconcileHardwareConfigurations(r); err != nil {
			// We do not want a stacktrace here, errs.Wrap already created
			// breadcrumb of errors to follow. Just sprintf with %v rather than %+v
			log.Info("Could not reconcile hardware configurations", "error", fmt.Sprintf("%v", err))
			//return reconcile.Result{}, errs.New("Reconciling failed")
			return reconcile.Result{Requeue: true}, nil
		}

	}

	return reconcile.Result{}, nil

}

func getDependencyFrom(specialresources *srov1beta1.SpecialResourceList, name string) (srov1beta1.SpecialResource, error) {

	log.Info("Looking for")

	for _, specialresource := range specialresources.Items {
		if specialresource.Name == name {
			return specialresource, nil
		}
	}

	return srov1beta1.SpecialResource{}, errs.New("Not found")
}

func createSpecialResourceFrom(r *SpecialResourceReconciler, name string) (srov1beta1.SpecialResource, error) {

	specialresource := srov1beta1.SpecialResource{}

	crpath := "/opt/sro/recipes/" + name
	manifests := getAssetsFrom(crpath)

	if len(manifests) == 0 {
		exitOnError(errs.New("Could not read CR " + name + "from lokal path"))
	}

	for _, manifest := range manifests {

		log.Info("Creating", "manifest", manifest.name)

		if err := createFromYAML(manifest.content, r, r.specialresource.Spec.Namespace); err != nil {
			log.Info("Cannot create, something went horribly wrong")
			exitOnError(err)
		}
		// Only one CR creation if they are more ignore all others
		// makes no sense to create multiple CRs for the same specialresource
		break
	}

	return specialresource, errs.New("Created new SpecialResource we need to Reconcile")
}
