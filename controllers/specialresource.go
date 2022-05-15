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
	log := ctrl.LoggerFrom(ctx)

	// Execute finalization logic if CR is being deleted
	isMarkedToBeDeleted := wi.SpecialResource.GetDeletionTimestamp() != nil
	if isMarkedToBeDeleted {
		log.Info("Marked to be deleted, reconciling finalizer")
		if err := r.StatusUpdater.SetAsProgressing(ctx, wi.SpecialResource, state.MarkedForDeletion, "CR is marked for deletion"); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to update SpecialResource's %s/%s status to Progressing: %w",
				wi.SpecialResource.Namespace, wi.SpecialResource.Name, err)
		}
		if err := r.Finalizer.Finalize(ctx, wi.SpecialResource); err != nil {
			return reconcile.Result{}, fmt.Errorf("unable to finalize SpecialResource %s/%s: %w", wi.SpecialResource.Namespace, wi.SpecialResource.Name, err)
		}
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
		if err := r.removeSpecialResource(ctx, wi.SpecialResource); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to remove SpecialResource %s/%s: %w", wi.SpecialResource.Namespace, wi.SpecialResource.Name, err)
		}
		return reconcile.Result{}, nil
	case operatorv1.Unmanaged:
		// The CR must be abandoned by the operator, leaving it in the last known status.
		// This is already filtered out, leaving for double safety.
		log.Info("ManagementState=Unmanaged; skipping")
		return reconcile.Result{}, nil
	default:
		return reconcile.Result{}, fmt.Errorf("ManagementState=%q; unhandled state", wi.SpecialResource.Spec.ManagementState)
	}

	if err := r.StatusUpdater.SetAsProgressing(ctx, wi.SpecialResource, state.Progressing, state.Progressing); err != nil {
		return reconcile.Result{}, fmt.Errorf("Failed to update SpecialResource's %s/%s status to Progressing: %w", wi.SpecialResource.Namespace, wi.SpecialResource.Name, err)
	}

	var err error
	wi.Chart, err = r.Helmer.Load(wi.SpecialResource.Spec.Chart)
	if err != nil {
		msg := fmt.Sprintf("failed to load helm chart %s", wi.SpecialResource.Spec.Chart.Name)
		if err := r.StatusUpdater.SetAsErrored(ctx, wi.SpecialResource, state.ChartFailure, fmt.Sprintf("%s: %v", msg, err)); err != nil {
			log.Info(utils.WarnString("Failed to update CR's status to Errored"), "error", err)
		}
		return reconcile.Result{}, fmt.Errorf("%s: %w", msg, err)
	}

	log.Info("Resolving dependencies", "count", len(wi.SpecialResource.Spec.Dependencies))

	// Only one level dependency support for now
	for _, dependency := range wi.SpecialResource.Spec.Dependencies {

		clog := log.WithValues("dependency", dependency.Name)
		cctx := ctrl.LoggerInto(ctx, clog)

		cchart, err := r.Helmer.Load(dependency.HelmChart)
		if err != nil {
			msg := fmt.Sprintf("failed to load dependency helm chart %s", dependency.HelmChart.Name)
			if suErr := r.StatusUpdater.SetAsErrored(cctx, wi.SpecialResource, state.DependencyChartFailure, fmt.Sprintf("%s: %v", msg, err)); suErr != nil {
				clog.Info(utils.WarnString("Failed to update CR's status to Errored"), "error", suErr)
			}
			return ctrl.Result{}, fmt.Errorf("%s: %w", msg, err)
		}

		// We save the dependency chain so we can restore specialresources
		// if one is deleted that is a dependency of another

		ins := types.NamespacedName{
			Namespace: os.Getenv("OPERATOR_NAMESPACE"),
			Name:      "special-resource-dependencies",
		}
		if err = r.Storage.UpdateConfigMapEntry(cctx, dependency.Name, wi.SpecialResource.Name, ins); err != nil {
			msg := "failed to store dependency information"
			if suErr := r.StatusUpdater.SetAsErrored(ctx, wi.SpecialResource, state.FailedToStoreDependencyInfo, fmt.Sprintf("%s: %v", msg, err)); suErr != nil {
				clog.Info(utils.WarnString("Failed to update CR's status to Errored"), "error", suErr)
			}
			return reconcile.Result{}, fmt.Errorf("%s: %w", msg, err)
		}

		var child srov1beta1.SpecialResource
		if child, err = getDependencyFrom(wi.AllSRs, dependency.Name); err != nil {
			clog.Info(utils.WarnString("Failed to find dependency in list of all SpecialResources"))
			if err = r.createSpecialResourceFrom(cctx, cchart, dependency.HelmChart); err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to create SpecialResource for dependency: %w", err)
			}
			// We need to fetch the newly created SpecialResources, reconciling
			return reconcile.Result{Requeue: true}, nil
		}

		child.Spec.Set = dependency.Set
		childWorkItem := wi.CreateForChild(&child, cchart)
		if err := r.ReconcileSpecialResourceChart(cctx, childWorkItem); err != nil {
			if suErr := r.StatusUpdater.SetAsErrored(cctx, &child, state.FailedToDeployDependencyChart, fmt.Sprintf("Failed to deploy dependency: %v", err)); suErr != nil {
				log.Info(utils.WarnString("Failed to update CR's status to Errored"), "error", suErr)
			}
			clog.Info(utils.WarnString("Failed to reconcile chart"), "error", err)
			return reconcile.Result{Requeue: true}, nil
		}

	}

	log.Info("Done resolving dependencies - reconciling main SpecialResource")
	if err := r.ReconcileSpecialResourceChart(ctx, wi); err != nil {
		msg := "failed to deploy SpecialResource's chart"
		if suErr := r.StatusUpdater.SetAsErrored(ctx, wi.SpecialResource, state.FailedToDeployChart, fmt.Sprintf("%s: %v", msg, err)); suErr != nil {
			log.Info(utils.WarnString("failed to update CR's status to Errored"), "error", suErr)
		}
		log.Info(utils.WarnString("RECONCILE REQUEUE: Could not reconcile chart for SpecialResource"), "error", err)
		return reconcile.Result{Requeue: true}, nil
	}

	if suErr := r.StatusUpdater.SetAsReady(ctx, wi.SpecialResource, state.Success, ""); suErr != nil {
		return reconcile.Result{}, fmt.Errorf("failed to update CR's status to Ready: %w", suErr)
	}
	return reconcile.Result{}, nil
}

func TemplateFragment(sr interface{}, runInfo *runtime.RuntimeInformation) error {
	spec, err := json.Marshal(sr)
	if err != nil {
		return fmt.Errorf("failed to marshal json: %w", err)
	}

	// We want the json representation of the data no the golang one
	info, err := json.MarshalIndent(runInfo, "", "  ")
	if err != nil {
		return fmt.Errorf("faile to marshalIndent json: %w", err)
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{},
	}

	if err = obj.UnmarshalJSON(info); err != nil {
		return fmt.Errorf("failed to unmarshal indent json: %s", err)
	}

	values := make(map[string]interface{})
	values["Values"] = obj.Object

	t, err := template.New("runtime").Parse(string(spec))
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	var buff bytes.Buffer

	if err = t.Execute(&buff, values); err != nil {
		return fmt.Errorf("failed to execute template %s: %w", t.Name(), err)
	}

	if err = json.Unmarshal(buff.Bytes(), sr); err != nil {
		return fmt.Errorf("failed to unmarshal proccessed template %s: %w", t.Name(), err)
	}
	return nil
}

func (r *SpecialResourceReconciler) ReconcileSpecialResourceChart(ctx context.Context, wi *WorkItem) error {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconciling chart", "chart", wi.Chart.Name())

	var err error
	wi.RunInfo, err = r.RuntimeAPI.GetRuntimeInformation(ctx, wi.SpecialResource)
	if err != nil {
		return fmt.Errorf("failed to get %s/%s runtime info: %w", wi.SpecialResource.Namespace, wi.SpecialResource.Name, err)
	}

	r.RuntimeAPI.LogRuntimeInformation(ctx, wi.RunInfo)

	for idx, dep := range wi.SpecialResource.Spec.Dependencies {
		if dep.Set.Object == nil {
			dep.Set.Object = make(map[string]interface{})
		}

		if err := unstructured.SetNestedField(dep.Set.Object, "Values", "kind"); err != nil {
			return fmt.Errorf("failed to set 'kind' nested field for obj %s: %w", dep.Name, err)
		}

		if err := unstructured.SetNestedField(dep.Set.Object, "sro.openshift.io/v1beta1", "apiVersion"); err != nil {
			return fmt.Errorf("failed to set 'apiVersion' nested field for obj %s: %w", dep.Name, err)
		}

		wi.SpecialResource.Spec.Dependencies[idx] = dep
	}

	if wi.SpecialResource.Spec.Set.Object == nil {
		wi.SpecialResource.Spec.Set.Object = make(map[string]interface{})
	}

	if err := unstructured.SetNestedField(wi.SpecialResource.Spec.Set.Object, "Values", "kind"); err != nil {
		return fmt.Errorf("failed to set 'kind' nested field for obj %s: %w", wi.SpecialResource.Name, err)
	}

	if err := unstructured.SetNestedField(wi.SpecialResource.Spec.Set.Object, "sro.openshift.io/v1beta1", "apiVersion"); err != nil {
		return fmt.Errorf("failed to set 'apiVersion' nested field for obj %s: %w", wi.SpecialResource.Name, err)
	}

	if err := TemplateFragment(wi.SpecialResource, wi.RunInfo); err != nil {
		return fmt.Errorf("failed to update template %s with runtim info: %w", wi.SpecialResource.Name, err)
	}

	if wi.SpecialResource.Spec.Set.Object == nil {
		wi.SpecialResource.Spec.Set.Object = make(map[string]interface{})
	}

	if err := unstructured.SetNestedField(wi.SpecialResource.Spec.Set.Object, "Values", "kind"); err != nil {
		return fmt.Errorf("failed to set 'kind' nested field for obj %s: %w", wi.SpecialResource.Name, err)
	}

	if err := unstructured.SetNestedField(wi.SpecialResource.Spec.Set.Object, "sro.openshift.io/v1beta1", "apiVersion"); err != nil {
		return fmt.Errorf("failed to set 'apiVersion' nested field for obj %s: %w", wi.SpecialResource.Name, err)
	}

	if err := TemplateFragment(&wi.SpecialResource.Spec.Set, wi.RunInfo); err != nil {
		return fmt.Errorf("failed to update template %s with runtim info: %w", wi.SpecialResource.Name, err)
	}

	// Add a finalizer to CR if it does not already have one
	if err := r.Finalizer.AddFinalizerToSpecialResource(ctx, wi.SpecialResource); err != nil {
		return fmt.Errorf("failed to add finalizer to SpecialResource %s/%s: %w", wi.SpecialResource.Namespace, wi.SpecialResource.Name, err)
	}

	// Reconcile the special resource chart
	if err := r.ReconcileChart(ctx, wi); err != nil {
		return fmt.Errorf("failed to reconcile SpecialResource %s/%s: %w", wi.SpecialResource.Namespace, wi.SpecialResource.Name, err)
	}
	return nil
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

func (r *SpecialResourceReconciler) createSpecialResourceFrom(ctx context.Context, ch *chart.Chart, dp helmerv1beta1.HelmChart) error {
	log := ctrl.LoggerFrom(ctx)

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

		_, err := r.KubeClient.CreateOrUpdate(ctx, &sr, noop)
		if err != nil {
			return fmt.Errorf("failed to create or update SpecialResource %s/%s: %w", sr.Namespace, sr.Name, err)
		}

		return nil
	}

	log.Info("Creating SpecialResource", "specialResourceName", ch.Files[idx].Name)

	if err := r.ResourceAPI.CreateFromYAML(
		ctx,
		ch.Files[idx].Data,
		false,
		&sr,
		sr.Name,
		sr.Namespace,
		sr.Spec.NodeSelector,
		"", "", SROwnedLabel); err != nil {
		return fmt.Errorf("failed to create SpecialResource %s/%s from yaml: %w", sr.Namespace, sr.Name, err)
	}

	return nil
}

func (r *SpecialResourceReconciler) removeSpecialResource(ctx context.Context, sr *srov1beta1.SpecialResource) error {
	if err := r.Finalizer.Finalize(ctx, sr); err != nil {
		return fmt.Errorf("failed to finalize SpecialResource %s/%s: %w", sr.Namespace, sr.Name, err)
	}
	return nil
}
