package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/template"

	srov1beta1 "github.com/openshift-psap/special-resource-operator/api/v1beta1"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"github.com/openshift-psap/special-resource-operator/pkg/exit"
	"github.com/openshift-psap/special-resource-operator/pkg/filter"
	"github.com/openshift-psap/special-resource-operator/pkg/helmer"
	helmerv1beta1 "github.com/openshift-psap/special-resource-operator/pkg/helmer/api/v1beta1"
	"github.com/openshift-psap/special-resource-operator/pkg/resource"
	"github.com/openshift-psap/special-resource-operator/pkg/slice"
	"github.com/openshift-psap/special-resource-operator/pkg/storage"
	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/chart"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// GetName of the special resource operator
func (r *SpecialResourceReconciler) GetName() string {
	return "special-resource-operator"
}

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
	r.Metrics.SetSpecialResourcesCreated(len(specialresources.Items))

	// Do not reconcile all SRs everytime, get the one were the request
	// came from, use the List for metrics and dashboard, we also need the
	// List to find the dependency
	var request int
	var found bool
	if request, found = FindSR(specialresources.Items, req.Name, "Name"); !found {
		// If we do not find the specialresource it might be deleted,
		// if it is a depdendency of another specialresource assign the
		// parent specialresource for processing.
		obj := types.NamespacedName{
			Namespace: os.Getenv("OPERATOR_NAMESPACE"),
			Name:      "special-resource-dependencies",
		}
		parent, err := storage.CheckConfigMapEntry(req.Name, obj)
		if err != nil {
			operatorStatusUpdate(&r.parent, fmt.Sprintf("%v", err))
			return reconcile.Result{}, err
		}
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
	if err != nil {
		operatorStatusUpdate(&r.parent, fmt.Sprintf("%v", err))
		return reconcile.Result{}, err
	}

	// Only one level dependency support for now
	for _, r.dependency = range r.parent.Spec.Dependencies {

		log = r.Log.WithName(color.Print(r.dependency.Name, color.Purple))
		log.Info("Getting Dependency")

		cchart, err := helmer.Load(r.dependency.HelmChart)
		exit.OnError(err)

		// We save the dependency chain so we can restore specialresources
		// if one is deleted that is a dependency of another

		ins := types.NamespacedName{
			Namespace: os.Getenv("OPERATOR_NAMESPACE"),
			Name:      "special-resource-dependencies",
		}
		err = storage.UpdateConfigMapEntry(r.dependency.Name, r.parent.Name, ins)
		if err != nil {
			operatorStatusUpdate(&r.parent, fmt.Sprintf("%v", err))
			return reconcile.Result{}, err
		}

		var child srov1beta1.SpecialResource
		// Assign the specialresource to the reconciler object
		if child, err = getDependencyFrom(specialresources, r.dependency.Name); err != nil {
			log.Info("Could not get SpecialResource dependency", "error", fmt.Sprintf("%v", err))
			if err = createSpecialResourceFrom(r, cchart, r.dependency.HelmChart); err != nil {
				log.Info("RECONCILE REQUEUE: Dependency creation failed ", "error", fmt.Sprintf("%v", err))
				return reconcile.Result{Requeue: true}, nil
			}
			// We need to fetch the newly created SpecialResources, reconciling
			return reconcile.Result{}, nil
		}
		if err := ReconcileSpecialResourceChart(r, child, cchart, r.dependency.Set); err != nil {
			// We do not want a stacktrace here, errors.Wrap already created
			// breadcrumb of errors to follow. Just sprintf with %v rather than %+v
			operatorStatusUpdate(&child, fmt.Sprintf("%v", err))
			log.Info("RECONCILE REQUEUE: Could not reconcile chart", "error", fmt.Sprintf("%v", err))
			//return reconcile.Result{}, errors.New("Reconciling failed")
			return reconcile.Result{Requeue: true}, nil
		}

	}

	log.Info("Reconciling Parent")
	if err := ReconcileSpecialResourceChart(r, r.parent, pchart, r.parent.Spec.Set); err != nil {
		// We do not want a stacktrace here, errors.Wrap already created
		// breadcrumb of errors to follow. Just sprintf with %v rather than %+v
		operatorStatusUpdate(&r.parent, fmt.Sprintf("%v", err))
		log.Info("RECONCILE REQUEUE: Could not reconcile chart", "error", fmt.Sprintf("%v", err))
		//return reconcile.Result{}, errors.New("Reconciling failed")
		return reconcile.Result{Requeue: true}, nil
	}

	log.Info("RECONCILE SUCCESS: All resources done")
	return reconcile.Result{}, nil
}

func TemplateFragmentOrDie(sr interface{}) {

	spec, err := json.Marshal(sr)
	exit.OnError(err)

	// We want the json representation of the data no the golang one
	info, err := json.MarshalIndent(RunInfo, "", "  ")
	exit.OnError(err)

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{},
	}
	err = obj.UnmarshalJSON(info)
	exit.OnError(err)

	values := make(map[string]interface{})
	values["Values"] = obj.Object

	t := template.Must(template.New("runtime").Parse(string(spec)))

	var buff bytes.Buffer
	err = t.Execute(&buff, values)
	exit.OnError(err)

	err = json.Unmarshal(buff.Bytes(), sr)
	exit.OnError(err)
}

func ReconcileSpecialResourceChart(r *SpecialResourceReconciler, sr srov1beta1.SpecialResource, chart *chart.Chart, values unstructured.Unstructured) error {

	r.specialresource = sr
	r.chart = *chart
	r.values = values

	log = r.Log.WithName(color.Print(r.specialresource.Name, color.Green))
	log.Info("Reconciling Chart")

	getRuntimeInformation(r)
	logRuntimeInformation()

	for idx, dep := range r.specialresource.Spec.Dependencies {
		if dep.Set.Object == nil {
			dep.Set.Object = make(map[string]interface{})
		}
		err := unstructured.SetNestedField(dep.Set.Object, "Values", "kind")
		exit.OnError(err)
		err = unstructured.SetNestedField(dep.Set.Object, "sro.openshift.io/v1beta1", "apiVersion")
		exit.OnError(err)

		r.specialresource.Spec.Dependencies[idx] = dep
	}

	if r.specialresource.Spec.Set.Object == nil {
		r.specialresource.Spec.Set.Object = make(map[string]interface{})
	}

	err := unstructured.SetNestedField(r.specialresource.Spec.Set.Object, "Values", "kind")
	exit.OnError(err)
	err = unstructured.SetNestedField(r.specialresource.Spec.Set.Object, "sro.openshift.io/v1beta1", "apiVersion")
	exit.OnError(err)

	TemplateFragmentOrDie(&r.specialresource)
	r.specialresource.DeepCopyInto(&RunInfo.SpecialResource)

	if r.values.Object == nil {
		r.values.Object = make(map[string]interface{})
	}
	err = unstructured.SetNestedField(r.values.Object, "Values", "kind")
	exit.OnError(err)
	err = unstructured.SetNestedField(r.values.Object, "sro.openshift.io/v1beta1", "apiVersion")
	exit.OnError(err)

	TemplateFragmentOrDie(&r.values)

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

func noop() error {
	return nil
}

func createSpecialResourceFrom(r *SpecialResourceReconciler, ch *chart.Chart, dp helmerv1beta1.HelmChart) error {

	vals := unstructured.Unstructured{}
	vals.SetKind("Values")
	vals.SetAPIVersion("sro.openshift.io/v1beta1")

	sr := srov1beta1.SpecialResource{}
	sr.Name = ch.Metadata.Name
	sr.Spec.Namespace = sr.Name
	sr.Spec.Chart.Name = sr.Name
	sr.Spec.Chart.Version = ch.Metadata.Version
	sr.Spec.Chart.Repository.Name = dp.Repository.Name
	sr.Spec.Chart.Repository.URL = dp.Repository.URL
	sr.Spec.Chart.Tags = make([]string, 0)
	sr.Spec.Set = vals
	sr.Spec.Dependencies = make([]srov1beta1.SpecialResourceDependency, 0)

	var idx int
	if idx = slice.FindCRFile(ch.Files, r.dependency.Name); idx == -1 {
		log.Info("Creating SpecialResource from template, cannot find it in charts directory")
		res, err := clients.Interface.CreateOrUpdate(context.TODO(), &sr, noop)
		exit.OnError(errors.Wrap(err, string(res)))
		return errors.New("Created new SpecialResource we need to Reconcile")
	}

	log.Info("Creating SpecialResource: " + ch.Files[idx].Name)

	if err := resource.CreateFromYAML(ch.Files[idx].Data,
		false,
		&r.specialresource,
		r.specialresource.Name,
		r.specialresource.Namespace,
		r.specialresource.Spec.NodeSelector,
		"", ""); err != nil {
		log.Info("Cannot create, something went horribly wrong")
		exit.OnError(err)
	}

	return errors.New("Created new SpecialResource we need to Reconcile")
}
