package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/template"

	operatorv1 "github.com/openshift/api/operator/v1"
	srov1beta1 "github.com/openshift/special-resource-operator/api/v1beta1"
	"github.com/openshift/special-resource-operator/internal/controllers/finalizers"
	"github.com/openshift/special-resource-operator/internal/controllers/state"
	helmerv1beta1 "github.com/openshift/special-resource-operator/pkg/helmer/api/v1beta1"
	"github.com/openshift/special-resource-operator/pkg/runtime"
	"github.com/openshift/special-resource-operator/pkg/utils"
	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/chart"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// SpecialResourcesReconcile Takes care of all specialresources in the cluster
func SpecialResourcesReconcile(ctx context.Context, r *SpecialResourceReconciler, req ctrl.Request) (ctrl.Result, error) {

	log = r.Log.WithName(utils.Print("reconcile: "+r.Filter.GetMode(), utils.Purple))

	specialresources := &srov1beta1.SpecialResourceList{}

	opts := []client.ListOption{}
	err := r.KubeClient.List(ctx, specialresources, opts...)
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
		parent, err := r.Storage.CheckConfigMapEntry(ctx, req.Name, obj)
		if err != nil {
			log.Error(err, "failed to check configmap entry", "configmap", obj.String())
			return reconcile.Result{}, err
		}
		request, found = FindSR(specialresources.Items, parent, "Name")
		if !found {
			return reconcile.Result{}, nil
		}
	}

	r.parent = &specialresources.Items[request]
	if suErr := r.StatusUpdater.SetAsProgressing(ctx, r.parent, state.Progressing, state.Progressing); suErr != nil {
		log.Error(suErr, "failed to update CR's status to Progressing")
		return reconcile.Result{}, suErr
	}

	// Execute finalization logic if CR is being deleted
	isMarkedToBeDeleted := r.parent.GetDeletionTimestamp() != nil
	if isMarkedToBeDeleted {
		r.specialresource = r.parent
		log.Info("Marked to be deleted, reconciling finalizer")
		if suErr := r.StatusUpdater.SetAsProgressing(ctx, r.parent, state.MarkedForDeletion, "CR is marked for deletion"); suErr != nil {
			log.Error(suErr, "failed to update CR's status to Progressing")
			return reconcile.Result{}, suErr
		}
		err = r.Finalizer.Finalize(ctx, r.specialresource)
		return reconcile.Result{}, err
	}

	log = r.Log.WithName(utils.Print(r.parent.Name, utils.Green))

	if r.parent.Name == "special-resource-preamble" {
		log.Info("Preamble done, waiting for specialresource requests")
		return reconcile.Result{}, nil
	}

	switch r.parent.Spec.ManagementState {
	case operatorv1.Force, operatorv1.Managed, "":
		// The CR must be managed by the operator.
		// "" is there for completion, as the ManagementState is optional.
		break
	case operatorv1.Removed:
		// The CR associated resources must be removed, even though the CR still exists.
		log.Info("ManagementState=Removed; finalizing the SpecialResource")
		err = removeSpecialResource(ctx, r)
		return reconcile.Result{}, err
	case operatorv1.Unmanaged:
		// The CR must be abandoned by the operator, leaving it in the last known status.
		// This is already filtered out, leaving for double safety.
		log.Info("ManagementState=Unmanaged; skipping")
		return reconcile.Result{}, nil
	default:
		return reconcile.Result{}, fmt.Errorf("ManagementState=%q; unhandled state", r.parent.Spec.ManagementState)
	}

	pchart, err := r.Helmer.Load(r.parent.Spec.Chart)
	if err != nil {
		if suErr := r.StatusUpdater.SetAsErrored(ctx, r.parent, state.ChartFailure, fmt.Sprintf("Failed to load Helm Chart: %v", err)); suErr != nil {
			log.Error(suErr, "failed to update CR's status to Errored")
		}
		return reconcile.Result{}, err
	}

	// Only one level dependency support for now
	for _, r.dependency = range r.parent.Spec.Dependencies {

		log = r.Log.WithName(utils.Print(r.dependency.Name, utils.Purple))

		cchart, err := r.Helmer.Load(r.dependency.HelmChart)
		if err != nil {
			if suErr := r.StatusUpdater.SetAsErrored(ctx, r.parent, state.DependencyChartFailure, fmt.Sprintf("Failed to load dependency Helm Chart: %v", err)); suErr != nil {
				log.Error(suErr, "failed to update CR's status to Errored")
			}
			return ctrl.Result{}, err
		}

		// We save the dependency chain so we can restore specialresources
		// if one is deleted that is a dependency of another

		ins := types.NamespacedName{
			Namespace: os.Getenv("OPERATOR_NAMESPACE"),
			Name:      "special-resource-dependencies",
		}
		if err = r.Storage.UpdateConfigMapEntry(ctx, r.dependency.Name, r.parent.Name, ins); err != nil {
			if suErr := r.StatusUpdater.SetAsErrored(ctx, r.parent, state.FailedToStoreDependencyInfo, fmt.Sprintf("Failed to store dependency information: %v", err)); suErr != nil {
				log.Error(suErr, "failed to update CR's status to Errored")
			}
			return reconcile.Result{}, err
		}

		var child srov1beta1.SpecialResource
		// Assign the specialresource to the reconciler object
		if child, err = getDependencyFrom(specialresources, r.dependency.Name); err != nil {
			log.Error(err, "Could not get SpecialResource dependency")
			if err = createSpecialResourceFrom(ctx, r, cchart, r.dependency.HelmChart); err != nil {
				log.Error(err, "RECONCILE REQUEUE: Dependency creation failed ")
				if suErr := r.StatusUpdater.SetAsErrored(ctx, r.parent, state.FailedToCreateDependencySR, fmt.Sprintf("Failed to create SR for dependency: %v", err)); suErr != nil {
					log.Error(suErr, "failed to update CR's status to Errored")
				}
				return reconcile.Result{Requeue: true}, nil
			}
			// We need to fetch the newly created SpecialResources, reconciling
			return reconcile.Result{}, nil
		}
		if err := ReconcileSpecialResourceChart(ctx, r, &child, cchart, r.dependency.Set); err != nil {
			// We do not want a stacktrace here, errors.Wrap already created
			// breadcrumb of errors to follow. Just sprintf with %v rather than %+v
			if suErr := r.StatusUpdater.SetAsErrored(ctx, &child, state.FailedToDeployDependencyChart, fmt.Sprintf("Failed to deploy dependency: %v", err)); suErr != nil {
				log.Error(suErr, "failed to update CR's status to Errored")
			}
			log.Error(err, "RECONCILE REQUEUE: Could not reconcile chart")
			//return reconcile.Result{}, errors.New("Reconciling failed")
			return reconcile.Result{Requeue: true}, nil
		}

	}

	if err := ReconcileSpecialResourceChart(ctx, r, r.parent, pchart, r.parent.Spec.Set); err != nil {
		// We do not want a stacktrace here, errors.Wrap already created
		// breadcrumb of errors to follow. Just sprintf with %v rather than %+v
		if suErr := r.StatusUpdater.SetAsErrored(ctx, r.parent, state.FailedToDeployChart, fmt.Sprintf("Failed to deploy SpecialResource's chart: %v", err)); suErr != nil {
			log.Error(suErr, "failed to update CR's status to Errored")
		}
		log.Error(err, "RECONCILE REQUEUE: Could not reconcile chart")
		//return reconcile.Result{}, errors.New("Reconciling failed")
		return reconcile.Result{Requeue: true}, nil
	}

	if suErr := r.StatusUpdater.SetAsReady(ctx, r.parent, state.Success, ""); suErr != nil {
		log.Error(suErr, "failed to update CR's status to Ready")
		return reconcile.Result{}, suErr
	}
	log.Info("RECONCILE SUCCESS")
	return reconcile.Result{}, nil
}

func TemplateFragment(sr interface{}, runInfo *runtime.RuntimeInformation) error {
	spec, err := json.Marshal(sr)
	if err != nil {
		return err
	}

	// We want the json representation of the data no the golang one
	info, err := json.MarshalIndent(runInfo, "", "  ")
	if err != nil {
		return err
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{},
	}

	if err = obj.UnmarshalJSON(info); err != nil {
		return err
	}

	values := make(map[string]interface{})
	values["Values"] = obj.Object

	t, err := template.New("runtime").Parse(string(spec))
	if err != nil {
		return err
	}

	var buff bytes.Buffer

	if err = t.Execute(&buff, values); err != nil {
		return err
	}

	return json.Unmarshal(buff.Bytes(), sr)
}

func ReconcileSpecialResourceChart(ctx context.Context, r *SpecialResourceReconciler, sr *srov1beta1.SpecialResource, chart *chart.Chart, values unstructured.Unstructured) error {

	r.specialresource = sr
	r.chart = *chart
	r.values = values

	log = r.Log.WithName(utils.Print(r.specialresource.Name, utils.Green))
	log.Info("Reconciling chart", "chart", r.chart.Name)

	if err := r.RuntimeAPI.GetRuntimeInformation(ctx, r.specialresource, &r.RunInfo); err != nil {
		return err
	}

	r.RuntimeAPI.LogRuntimeInformation(&r.RunInfo)

	for idx, dep := range r.specialresource.Spec.Dependencies {
		if dep.Set.Object == nil {
			dep.Set.Object = make(map[string]interface{})
		}

		if err := unstructured.SetNestedField(dep.Set.Object, "Values", "kind"); err != nil {
			return err
		}

		if err := unstructured.SetNestedField(dep.Set.Object, "sro.openshift.io/v1beta1", "apiVersion"); err != nil {
			return err
		}

		r.specialresource.Spec.Dependencies[idx] = dep
	}

	if r.specialresource.Spec.Set.Object == nil {
		r.specialresource.Spec.Set.Object = make(map[string]interface{})
	}

	if err := unstructured.SetNestedField(r.specialresource.Spec.Set.Object, "Values", "kind"); err != nil {
		return err
	}

	if err := unstructured.SetNestedField(r.specialresource.Spec.Set.Object, "sro.openshift.io/v1beta1", "apiVersion"); err != nil {
		return err
	}

	if err := TemplateFragment(&r.specialresource, &r.RunInfo); err != nil {
		return err
	}

	r.specialresource.DeepCopyInto(&r.RunInfo.SpecialResource)

	if r.values.Object == nil {
		r.values.Object = make(map[string]interface{})
	}
	if err := unstructured.SetNestedField(r.values.Object, "Values", "kind"); err != nil {
		return err
	}

	if err := unstructured.SetNestedField(r.values.Object, "sro.openshift.io/v1beta1", "apiVersion"); err != nil {
		return err
	}

	if err := TemplateFragment(&r.values, &r.RunInfo); err != nil {
		return err
	}

	// Add a finalizer to CR if it does not already have one
	if !utils.StringSliceContains(r.specialresource.GetFinalizers(), finalizers.FinalizerString) {
		if err := r.Finalizer.AddToSpecialResource(ctx, r.specialresource); err != nil {
			log.Error(err, "Failed to add finalizer")
			return err
		}
	}

	// Reconcile the special resource chart
	return ReconcileChart(ctx, r)
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
	if idx, found := FindSR(specialresources.Items, name, "Name"); found {
		return specialresources.Items[idx], nil
	}
	return srov1beta1.SpecialResource{}, errors.New("Not found")
}

func noop() error {
	return nil
}

func createSpecialResourceFrom(ctx context.Context, r *SpecialResourceReconciler, ch *chart.Chart, dp helmerv1beta1.HelmChart) error {

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
	if idx = utils.FindCRFile(ch.Files, r.dependency.Name); idx == -1 {
		log.Info("Creating SpecialResource from template, cannot find it in charts directory")

		res, err := r.KubeClient.CreateOrUpdate(ctx, &sr, noop)
		if err != nil {
			return fmt.Errorf("%s: %w", res, err)
		}

		return errors.New("Created new SpecialResource we need to Reconcile")
	}

	log.Info("Creating SpecialResource", "name", ch.Files[idx].Name)

	if err := r.Creator.CreateFromYAML(
		ctx,
		ch.Files[idx].Data,
		false,
		r.specialresource,
		r.specialresource.Name,
		r.specialresource.Namespace,
		r.specialresource.Spec.NodeSelector,
		"", ""); err != nil {
		return err
	}
	return errors.New("Created new SpecialResource we need to Reconcile")
}

func removeSpecialResource(ctx context.Context, r *SpecialResourceReconciler) error {
	r.specialresource = r.parent
	return r.Finalizer.Finalize(ctx, r.specialresource)
}
