package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/template"

	"github.com/go-logr/logr"
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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// SpecialResourcesReconcile Takes care of all specialresources in the cluster
func (r *SpecialResourceReconciler) SpecialResourcesReconcile(ctx context.Context, wi *WorkItem) (ctrl.Result, error) {
	log := wi.Log

	// Execute finalization logic if CR is being deleted
	isMarkedToBeDeleted := wi.SpecialResource.GetDeletionTimestamp() != nil
	if isMarkedToBeDeleted {
		log.Info("Marked to be deleted, reconciling finalizer")
		if suErr := r.StatusUpdater.SetAsProgressing(ctx, wi.SpecialResource, state.MarkedForDeletion, "CR is marked for deletion"); suErr != nil {
			log.Error(suErr, "failed to update CR's status to Progressing")
			return reconcile.Result{}, suErr
		}
		return reconcile.Result{}, r.Finalizer.Finalize(ctx, wi.SpecialResource)
	}

	if wi.SpecialResource.Name == "special-resource-preamble" {
		log.Info("Preamble done, waiting for specialresource requests")
		return reconcile.Result{}, nil
	}

	switch wi.SpecialResource.Spec.ManagementState {
	case operatorv1.Force, operatorv1.Managed, "":
		// The CR must be managed by the operator.
		// "" is there for completion, as the ManagementState is optional.
		break
	case operatorv1.Removed:
		// The CR associated resources must be removed, even though the CR still exists.
		log.Info("ManagementState=Removed; finalizing the SpecialResource")
		return reconcile.Result{}, r.removeSpecialResource(ctx, wi.SpecialResource)
	case operatorv1.Unmanaged:
		// The CR must be abandoned by the operator, leaving it in the last known status.
		// This is already filtered out, leaving for double safety.
		log.Info("ManagementState=Unmanaged; skipping")
		return reconcile.Result{}, nil
	default:
		return reconcile.Result{}, fmt.Errorf("ManagementState=%q; unhandled state", wi.SpecialResource.Spec.ManagementState)
	}

	if suErr := r.StatusUpdater.SetAsProgressing(ctx, wi.SpecialResource, state.Progressing, state.Progressing); suErr != nil {
		log.Error(suErr, "failed to update CR's status to Progressing")
		return reconcile.Result{}, suErr
	}

	var err error
	wi.Chart, err = r.Helmer.Load(wi.SpecialResource.Spec.Chart)
	if err != nil {
		if suErr := r.StatusUpdater.SetAsErrored(ctx, wi.SpecialResource, state.ChartFailure, fmt.Sprintf("Failed to load Helm Chart: %v", err)); suErr != nil {
			log.Error(suErr, "failed to update CR's status to Errored")
		}
		return reconcile.Result{}, err
	}

	log.Info("Resolving dependencies")

	// Only one level dependency support for now
	for _, dependency := range wi.SpecialResource.Spec.Dependencies {

		clog := log.WithName(utils.Print(dependency.Name, utils.Purple))

		cchart, err := r.Helmer.Load(dependency.HelmChart)
		if err != nil {
			if suErr := r.StatusUpdater.SetAsErrored(ctx, wi.SpecialResource, state.DependencyChartFailure, fmt.Sprintf("Failed to load dependency Helm Chart: %v", err)); suErr != nil {
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
		if err = r.Storage.UpdateConfigMapEntry(ctx, dependency.Name, wi.SpecialResource.Name, ins); err != nil {
			if suErr := r.StatusUpdater.SetAsErrored(ctx, wi.SpecialResource, state.FailedToStoreDependencyInfo, fmt.Sprintf("Failed to store dependency information: %v", err)); suErr != nil {
				log.Error(suErr, "failed to update CR's status to Errored")
			}
			return reconcile.Result{}, err
		}

		var child srov1beta1.SpecialResource
		if child, err = getDependencyFrom(wi.AllSRs, dependency.Name); err != nil {
			clog.Info("Failed to find dependency in list of all SpecialResources")
			if err = r.createSpecialResourceFrom(ctx, clog, cchart, dependency.HelmChart); err != nil {
				clog.Error(err, "Failed to create SpecialResource for dependency")
				return reconcile.Result{}, err
			}
			// We need to fetch the newly created SpecialResources, reconciling
			return reconcile.Result{Requeue: true}, nil
		}

		child.Spec.Set = dependency.Set
		childWorkItem := wi.CreateForChild(&child, cchart)
		if err := r.ReconcileSpecialResourceChart(ctx, childWorkItem); err != nil {
			if suErr := r.StatusUpdater.SetAsErrored(ctx, &child, state.FailedToDeployDependencyChart, fmt.Sprintf("Failed to deploy dependency: %v", err)); suErr != nil {
				log.Error(suErr, "failed to update CR's status to Errored")
			}
			clog.Error(err, "Failed to reconcile chart")
			return reconcile.Result{Requeue: true}, nil
		}

	}

	log.Info("Done resolving dependencies - reconciling main SpecialResource")
	if err := r.ReconcileSpecialResourceChart(ctx, wi); err != nil {
		if suErr := r.StatusUpdater.SetAsErrored(ctx, wi.SpecialResource, state.FailedToDeployChart, fmt.Sprintf("Failed to deploy SpecialResource's chart: %v", err)); suErr != nil {
			log.Error(suErr, "failed to update CR's status to Errored")
		}
		log.Error(err, "RECONCILE REQUEUE: Could not reconcile chart")
		return reconcile.Result{Requeue: true}, nil
	}

	if suErr := r.StatusUpdater.SetAsReady(ctx, wi.SpecialResource, state.Success, ""); suErr != nil {
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

func (r *SpecialResourceReconciler) ReconcileSpecialResourceChart(ctx context.Context, wi *WorkItem) error {
	wi.Log.Info("Reconciling chart", "chart", wi.Chart.Name)

	var err error
	wi.RunInfo, err = r.RuntimeAPI.GetRuntimeInformation(ctx, wi.SpecialResource)
	if err != nil {
		return err
	}

	r.RuntimeAPI.LogRuntimeInformation(wi.RunInfo)

	for idx, dep := range wi.SpecialResource.Spec.Dependencies {
		if dep.Set.Object == nil {
			dep.Set.Object = make(map[string]interface{})
		}

		if err := unstructured.SetNestedField(dep.Set.Object, "Values", "kind"); err != nil {
			return err
		}

		if err := unstructured.SetNestedField(dep.Set.Object, "sro.openshift.io/v1beta1", "apiVersion"); err != nil {
			return err
		}

		wi.SpecialResource.Spec.Dependencies[idx] = dep
	}

	if wi.SpecialResource.Spec.Set.Object == nil {
		wi.SpecialResource.Spec.Set.Object = make(map[string]interface{})
	}

	if err := unstructured.SetNestedField(wi.SpecialResource.Spec.Set.Object, "Values", "kind"); err != nil {
		return err
	}

	if err := unstructured.SetNestedField(wi.SpecialResource.Spec.Set.Object, "sro.openshift.io/v1beta1", "apiVersion"); err != nil {
		return err
	}

	if err := TemplateFragment(wi.SpecialResource, wi.RunInfo); err != nil {
		return err
	}

	if wi.SpecialResource.Spec.Set.Object == nil {
		wi.SpecialResource.Spec.Set.Object = make(map[string]interface{})
	}

	if err := unstructured.SetNestedField(wi.SpecialResource.Spec.Set.Object, "Values", "kind"); err != nil {
		return err
	}

	if err := unstructured.SetNestedField(wi.SpecialResource.Spec.Set.Object, "sro.openshift.io/v1beta1", "apiVersion"); err != nil {
		return err
	}

	if err := TemplateFragment(&wi.SpecialResource.Spec.Set, wi.RunInfo); err != nil {
		return err
	}

	// Add a finalizer to CR if it does not already have one
	if !utils.StringSliceContains(wi.SpecialResource.GetFinalizers(), finalizers.FinalizerString) {
		if err := r.Finalizer.AddToSpecialResource(ctx, wi.SpecialResource); err != nil {
			wi.Log.Error(err, "Failed to add finalizer")
			return err
		}
	}

	// Reconcile the special resource chart
	return r.ReconcileChart(ctx, wi)
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

func (r *SpecialResourceReconciler) createSpecialResourceFrom(ctx context.Context, log logr.Logger, ch *chart.Chart, dp helmerv1beta1.HelmChart) error {

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
	if idx = utils.FindCRFile(ch.Files, sr.Name); idx == -1 {
		log.Info("Creating SpecialResource from template, cannot find it in charts directory")

		res, err := r.KubeClient.CreateOrUpdate(ctx, &sr, noop)
		if err != nil {
			return fmt.Errorf("%s: %w", res, err)
		}

		return nil
	}

	log.Info("Creating SpecialResource", "name", ch.Files[idx].Name)

	if err := r.ResourceAPI.CreateFromYAML(
		ctx,
		ch.Files[idx].Data,
		false,
		&sr,
		sr.Name,
		sr.Namespace,
		sr.Spec.NodeSelector,
		"", ""); err != nil {
		return err
	}

	return nil
}

func (r *SpecialResourceReconciler) removeSpecialResource(ctx context.Context, sr *srov1beta1.SpecialResource) error {
	return r.Finalizer.Finalize(ctx, sr)
}
