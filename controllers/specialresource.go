package controllers

import (
	"context"
	"fmt"

	srov1beta1 "github.com/openshift-psap/special-resource-operator/api/v1beta1"
	"github.com/openshift-psap/special-resource-operator/pkg/cache"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"github.com/openshift-psap/special-resource-operator/pkg/dependencies"
	"github.com/openshift-psap/special-resource-operator/pkg/exit"
	"github.com/openshift-psap/special-resource-operator/pkg/filter"
	"github.com/openshift-psap/special-resource-operator/pkg/helmer"
	"github.com/openshift-psap/special-resource-operator/pkg/metrics"
	"github.com/openshift-psap/special-resource-operator/pkg/slice"
	"github.com/openshift-psap/special-resource-operator/pkg/upgrade"
	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/chart"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// GetName of the special resource operator
func (r *SpecialResourceReconciler) GetName() string {
	return "special-resource-operator"
}

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
// +kubebuilder:rbac:groups=core,resources=events,verbs=list;watch;create;update;patch;delete;get
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
// +kubebuilder:rbac:groups=config.openshift.io,resources=clusteroperators,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=config.openshift.io,resources=clusteroperators/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cert-manager.io,resources=issuers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=create;patch;delete
// +kubebuilder:rbac:groups=core,resources=services/finalizers,verbs=create;delete;get;list;update;patch;delete;watch
// +kubebuilder:rbac:groups=apps,resources=deployments/finalizers,resourceNames=shipwright-build,verbs=update
// +kubebuilder:rbac:groups=apps,resources=replicasets,verbs=create;delete;get;list;patch;update;watch;get
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=shipwright.io,resources=*,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=shipwright.io,resources=buildruns,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=shipwright.io,resources=buildstrategies,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=shipwright.io,resources=clusterbuildstrategies,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=tekton.dev,resources=taskruns,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=tekton.dev,resources=tasks,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=storage.k8s.io,resources=volumeattachments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=snapshot.storage.k8s.io,resources=volumesnapshotclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=snapshot.storage.k8s.io,resources=volumesnapshots,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=snapshot.storage.k8s.io,resources=volumesnapshotcontents,verbs=create;get;list;watch;update;delete
// +kubebuilder:rbac:groups=snapshot.storage.k8s.io,resources=volumesnapshots/status,verbs=create;get;list;watch;update;delete
// +kubebuilder:rbac:groups=snapshot.storage.k8s.io,resources=volumesnapshotcontents/status,verbs=create;get;list;watch;update;delete
// +kubebuilder:rbac:groups=csi.storage.k8s.io,resources=csidrivers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims/status,verbs=get;list;watch;create;delete;update;patch

// SpecialResourcesReconcile Takes care of all specialresources in the cluster
func SpecialResourcesReconcile(r *SpecialResourceReconciler, req ctrl.Request) (ctrl.Result, error) {

	log = r.Log.WithName(color.Print("reconcile: "+filter.Mode, color.Purple))

	log.Info("Reconciling SpecialResource(s) in all Namespaces")

	specialresources := &srov1beta1.SpecialResourceList{}

	opts := []client.ListOption{}
	err := clients.Interface.List(context.TODO(), specialresources, opts...)
	if err != nil {
		// Error reading the object - requeue the request.
		// This should never happen
		return reconcile.Result{}, err
	}

	// Set specialResourcesCreated metric to the number of specialresources
	metrics.SetSpecialResourcesCreated(len(specialresources.Items))

	// Do not reconcile all SRs everytime, get the one were the request
	// came from, use the List for metrics and dashboard, we also need the
	// List to find the dependency
	var request int
	var found bool
	if request, found = FindSR(specialresources.Items, req.Name, "Name"); !found {
		// If we do not find the specialresource it might be deleted,
		// if it is a depdendency of another specialresource assign the
		// parent specialresource for processing.
		parent := dependencies.CheckConfigMap(req.Name)
		request, found = FindSR(specialresources.Items, parent, "Name")
		if !found {
			return reconcile.Result{}, nil
		}
	}

	r.parent = specialresources.Items[request]

	// Execute finalization logic if CR is being deleted
	isMarkedToBeDeleted := r.parent.GetDeletionTimestamp() != nil
	if isMarkedToBeDeleted {
		r.specialresource = r.parent
		log.Info("Marked to be deleted, reconciling finalizer")
		err = reconcileFinalizers(r)
		return reconcile.Result{}, err
	}

	log = r.Log.WithName(color.Print(r.parent.Name, color.Green))

	if r.parent.Name == "special-resource-preamble" {
		log.Info("Preamble done, waiting for specialresource requests")
		return reconcile.Result{}, nil
	}

	log.Info("Resolving Dependencies")

	pchart, err := helmer.Load(r.parent.Spec.Chart)
	exit.OnError(err)

	// Only one level dependency support for now
	for _, r.dependency = range r.parent.Spec.Dependencies {

		log = r.Log.WithName(color.Print(r.dependency.Name, color.Purple))
		log.Info("Getting Dependency")

		cchart, err := helmer.Load(r.dependency)
		exit.OnError(err)

		// We save the dependency chain so we can restore specialresources
		// if one is deleted that is a dependency of another
		dependencies.UpdateConfigMap(r.parent.Name, r.dependency.Name)

		var child srov1beta1.SpecialResource
		// Assign the specialresource to the reconciler object
		if child, err = getDependencyFrom(specialresources, r.dependency.Name); err != nil {
			log.Info("Could not get SpecialResource dependency", "error", fmt.Sprintf("%v", err))
			if _, err = createSpecialResourceFrom(r, cchart.Files, r.dependency.Name); err != nil {
				log.Info("RECONCILE REQUEUE: Dependency creation failed ", "error", fmt.Sprintf("%v", err))
				return reconcile.Result{Requeue: true}, nil
			}
			// We need to fetch the newly created SpecialResources, reconciling
			return reconcile.Result{}, nil
		}
		if err := ReconcileSpecialResourceChart(r, child, cchart); err != nil {
			// We do not want a stacktrace here, errors.Wrap already created
			// breadcrumb of errors to follow. Just sprintf with %v rather than %+v
			log.Info("RECONCILE REQUEUE: Could not reconcile chart", "error", fmt.Sprintf("%v", err))
			//return reconcile.Result{}, errors.New("Reconciling failed")
			return reconcile.Result{Requeue: true}, nil
		}

	}

	if err := ReconcileSpecialResourceChart(r, r.parent, pchart); err != nil {
		// We do not want a stacktrace here, errors.Wrap already created
		// breadcrumb of errors to follow. Just sprintf with %v rather than %+v
		log.Info("RECONCILE REQUEUE: Could not reconcile chart", "error", fmt.Sprintf("%v", err))
		//return reconcile.Result{}, errors.New("Reconciling failed")
		return reconcile.Result{Requeue: true}, nil
	}

	log.Info("RECONCILE SUCCESS: All resources done")
	return reconcile.Result{}, nil
}

func ReconcileSpecialResourceChart(r *SpecialResourceReconciler, sr srov1beta1.SpecialResource, chart *chart.Chart) error {

	r.specialresource = sr
	r.chart = *chart

	log = r.Log.WithName(color.Print(r.specialresource.Name, color.Green))
	log.Info("Reconciling")

	// This is specific for the specialresource we need to update the nodes
	// and the upgradeinfo
	err := cache.Nodes(r.specialresource.Spec.NodeSelector, true)
	exit.OnError(errors.Wrap(err, "Failed to cache nodes"))

	RunInfo.ClusterUpgradeInfo, err = upgrade.ClusterInfo()
	exit.OnError(errors.Wrap(err, "Failed to get upgrade info"))

	// Add a finalizer to CR if it does not already have one
	if !contains(r.specialresource.GetFinalizers(), specialresourceFinalizer) {
		if err := addFinalizer(r); err != nil {
			log.Info("Failed to add finalizer", "error", fmt.Sprintf("%v", err))
			return err
		}
	}

	// Reconcile the special resource chart
	return ReconcileChart(r)
}

func FindSR(a []srov1beta1.SpecialResource, x string, by string) (int, bool) {
	for i, n := range a {
		if by == "Name" {
			if x == n.GetName() {
				return i, true
			}
		}
	}
	return -1, false
}

func getDependencyFrom(specialresources *srov1beta1.SpecialResourceList, name string) (srov1beta1.SpecialResource, error) {

	log.Info("Looking for SpecialResource in fetched List (all namespaces)")
	if idx, found := FindSR(specialresources.Items, name, "Name"); found {
		return specialresources.Items[idx], nil
	}

	return srov1beta1.SpecialResource{}, errors.New("Not found")
}

func createSpecialResourceFrom(r *SpecialResourceReconciler, cr []*chart.File, name string) (srov1beta1.SpecialResource, error) {

	specialresource := srov1beta1.SpecialResource{}

	var idx int
	if idx = slice.FindCRFile(cr, r.dependency.Name); idx == -1 {
		return specialresource, errors.New("Create the SpecialResource no your own, cannot find it in charts directory")
	}

	log.Info("Creating SpecialResource: " + cr[idx].Name)

	if err := createFromYAML(cr[idx].Data, r, r.specialresource.Spec.Namespace, "", ""); err != nil {
		log.Info("Cannot create, something went horribly wrong")
		exit.OnError(err)
	}

	return specialresource, errors.New("Created new SpecialResource we need to Reconcile")
}
