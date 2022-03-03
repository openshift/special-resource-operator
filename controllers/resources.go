package controllers

import (
	"context"
	"fmt"
	"regexp"
	"sort"

	s "github.com/openshift/special-resource-operator/internal/controllers/state"
	"github.com/openshift/special-resource-operator/pkg/state"
	"github.com/openshift/special-resource-operator/pkg/upgrade"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

var (
	affineRegex = regexp.MustCompile(`(?m)^\s+specialresource\.openshift\.io/kernel-affine:.*$`)
)

func (r *SpecialResourceReconciler) createImagePullerRoleBinding(ctx context.Context, wi *WorkItem) error {
	rb := &unstructured.Unstructured{}
	rb.SetAPIVersion("rbac.authorization.k8s.io/v1")
	rb.SetKind("RoleBinding")

	namespacedName := types.NamespacedName{Namespace: wi.SpecialResource.Spec.Namespace, Name: "system:image-pullers"}
	err := r.KubeClient.Get(ctx, namespacedName, rb)
	if apierrors.IsNotFound(err) {
		wi.Log.Error(err, "Warning: RoleBinding not found", "name", namespacedName)
		return nil
	} else if err != nil {
		return errors.Wrap(err, "Error checking for image-pullers roleBinding")
	}

	newSubject := make(map[string]interface{})
	newSubjects := make([]interface{}, 0)

	newSubject["kind"] = "ServiceAccount"
	newSubject["name"] = "builder"
	newSubject["namespace"] = wi.SpecialResource.Spec.Namespace

	if apierrors.IsNotFound(err) {
		wi.Log.Info("ImagePuller RoleBinding not found, creating")
		rb.SetName("system:image-puller")
		rb.SetNamespace(wi.SpecialResource.Spec.Namespace)

		if err = unstructured.SetNestedField(rb.Object, "rbac.authorization.k8s.io", "roleRef", "apiGroup"); err != nil {
			return err
		}

		if err = unstructured.SetNestedField(rb.Object, "ClusterRole", "roleRef", "kind"); err != nil {
			return err
		}

		if err = unstructured.SetNestedField(rb.Object, "system:image-puller", "roleRef", "name"); err != nil {
			return err
		}

		newSubjects = append(newSubjects, newSubject)

		if err = unstructured.SetNestedSlice(rb.Object, newSubjects, "subjects"); err != nil {
			return err
		}

		if err = r.KubeClient.Create(ctx, rb); err != nil {
			return fmt.Errorf("couldn't Create Resource: %w", err)
		}

		return nil
	}

	if apierrors.IsForbidden(err) {
		return fmt.Errorf("forbidden - check Role, ClusterRole and Bindings for operator: %w", err)
	}

	if err != nil {
		return fmt.Errorf("unexpected error: %w", err)
	}

	oldSubjects, _, err := unstructured.NestedSlice(rb.Object, "subjects")
	if err != nil {
		return err
	}

	for _, subject := range oldSubjects {
		switch subject := subject.(type) {
		case map[string]interface{}:
			namespace, _, err := unstructured.NestedString(subject, "namespace")
			if err != nil {
				return err
			}

			if namespace == wi.SpecialResource.Spec.Namespace {
				return nil
			}
		default:
			wi.Log.Info("subject", "DEFAULT NOT THE CORRECT TYPE", subject)
		}
	}

	oldSubjects = append(oldSubjects, newSubject)

	if err = unstructured.SetNestedSlice(rb.Object, oldSubjects, "subjects"); err != nil {
		return err
	}

	if err = r.KubeClient.Update(ctx, rb); err != nil {
		return fmt.Errorf("couldn't Update Resource: %w", err)
	}

	return nil
}

// ReconcileChartStates Reconcile Hardware States
func (r *SpecialResourceReconciler) ReconcileChartStates(ctx context.Context, wi *WorkItem) error {

	basicChart := *wi.Chart
	basicChart.Templates = []*chart.File{}
	stateYAMLS := []*chart.File{}
	statelessYAMLS := []*chart.File{}

	// differentiate between stateful YAMLs,
	// stateless YAMLS, and named templates, which should be run with both
	for _, template := range wi.Chart.Templates {
		switch {
		case r.Assets.ValidStateName(template.Name):
			stateYAMLS = append(stateYAMLS, template)
		case r.Assets.NamedTemplate(template.Name):
			basicChart.Templates = append(basicChart.Templates, template)
		default:
			statelessYAMLS = append(statelessYAMLS, template)
		}
	}

	// sort statefull yaml by names
	sort.Slice(stateYAMLS, func(i, j int) bool {
		return stateYAMLS[i].Name < stateYAMLS[j].Name
	})

	for _, stateYAML := range stateYAMLS {
		wi.Log.Info("Executing", "State", stateYAML.Name)
		if suErr := r.StatusUpdater.SetAsProgressing(ctx, wi.SpecialResource, s.HandlingState, fmt.Sprintf("Working on: %s", stateYAML.Name)); suErr != nil {
			wi.Log.Error(suErr, "failed to update CR's status to Progressing")
			return suErr
		}

		if wi.SpecialResource.Spec.Debug {
			wi.Log.Info("Debug active. Showing YAML contents", "name", stateYAML.Name, "data", stateYAML.Data)
		}

		// Every YAML is one state, we generate the name of the
		// state special-resource + first 4 digits of the state
		// e.g.: simple-kmod-0000 this can be used for scheduling or
		// affinity, anti-affinity
		state.GenerateName(stateYAML, wi.SpecialResource.Name)

		step := basicChart
		step.Templates = append(step.Templates, stateYAML)

		// We are kernel-affine if the yamlSpec uses kernel-affine label.
		// then we need to replicate the object and set a name + os + kernel version
		kernelAffine := affineRegex.Match(stateYAML.Data)

		// var replicas is to keep track of the number of replicas
		// and either to break or continue the for looop
		var replicas int

		// The cluster has more then one kernel version running
		// we're replicating the driver-container DaemonSet to
		// the number of kernel versions running in the cluster
		if len(wi.RunInfo.ClusterUpgradeInfo) == 0 {
			return errors.New("no KernelVersion detected, something is wrong")
		}

		var version upgrade.NodeVersion
		for wi.RunInfo.KernelFullVersion, version = range wi.RunInfo.ClusterUpgradeInfo {

			wi.RunInfo.ClusterVersionMajorMinor = version.ClusterVersion
			wi.RunInfo.OperatingSystemDecimal = version.OSVersion
			wi.RunInfo.OperatingSystemMajorMinor = version.OSMajorMinor
			wi.RunInfo.OperatingSystemMajor = version.OSMajor
			wi.RunInfo.DriverToolkitImage = version.DriverToolkit.ImageURL

			var err error

			step.Values, err = chartutil.CoalesceValues(&step, wi.Values.Object)
			if err != nil {
				return err
			}

			rinfo, err := runtime.DefaultUnstructuredConverter.ToUnstructured(wi.RunInfo)
			if err != nil {
				return err
			}

			step.Values, err = chartutil.CoalesceValues(&step, rinfo)
			if err != nil {
				return err
			}

			if wi.SpecialResource.Spec.Debug {
				d, _ := yaml.Marshal(step.Values)
				wi.Log.Info("Debug active. Showing YAML values", "values", d)
			}

			err = r.Helmer.Run(
				ctx,
				step,
				step.Values,
				wi.SpecialResource,
				wi.SpecialResource.Name,
				wi.SpecialResource.Spec.Namespace,
				wi.SpecialResource.Spec.NodeSelector,
				wi.RunInfo.KernelFullVersion,
				wi.RunInfo.OperatingSystemDecimal,
				wi.SpecialResource.Spec.Debug)

			replicas += 1

			// If the first replica fails we want to create all remaining
			// ones for parallel startup, otherwise we would wait for the first
			// then for the second etc.
			if err != nil && replicas == len(wi.RunInfo.ClusterUpgradeInfo) {
				r.Metrics.SetCompletedState(wi.SpecialResource.Name, stateYAML.Name, 0)
				return fmt.Errorf("failed to create state %s: %w ", stateYAML.Name, err)
			}

			// We're always doing one run to create a non kernel affine resource
			if !kernelAffine {
				break
			}
		}

		r.Metrics.SetCompletedState(wi.SpecialResource.Name, stateYAML.Name, 1)
		// If resource available, label the nodes according to the current state
		// if e.g driver-container ready -> specialresource.openshift.io/driver-container:ready
		if err := r.labelNodesAccordingToState(ctx, wi.Log, wi.SpecialResource.Spec.NodeSelector); err != nil {
			return err
		}
	}

	// We're done with states, now execute the part of the chart without
	// states we need to reconcile the nostate Chart
	nostate := basicChart
	nostate.Templates = append(nostate.Templates, statelessYAMLS...)
	var err error
	nostate.Values, err = chartutil.CoalesceValues(&nostate, wi.Values.Object)
	if err != nil {
		return err
	}

	rinfo, err := runtime.DefaultUnstructuredConverter.ToUnstructured(wi.RunInfo)
	if err != nil {
		return err
	}

	nostate.Values, err = chartutil.CoalesceValues(&nostate, rinfo)
	if err != nil {
		return err
	}

	return r.Helmer.Run(
		ctx,
		nostate,
		nostate.Values,
		wi.SpecialResource,
		wi.SpecialResource.Name,
		wi.SpecialResource.Spec.Namespace,
		wi.SpecialResource.Spec.NodeSelector,
		wi.RunInfo.KernelFullVersion,
		wi.RunInfo.OperatingSystemDecimal,
		false)
}

func (r *SpecialResourceReconciler) createSpecialResourceNamespace(ctx context.Context, wi *WorkItem) error {

	ns := []byte(`apiVersion: v1
kind: Namespace
metadata:
  annotations:
    specialresource.openshift.io/wait: "true"
    openshift.io/cluster-monitoring: "true"
  name: `)

	if wi.SpecialResource.Spec.Namespace != "" {
		add := []byte(wi.SpecialResource.Spec.Namespace)
		ns = append(ns, add...)
	} else {
		wi.SpecialResource.Spec.Namespace = wi.SpecialResource.Name
		add := []byte(wi.SpecialResource.Spec.Namespace)
		ns = append(ns, add...)
	}

	if err := r.Creator.CreateFromYAML(ctx, ns, false, wi.SpecialResource, wi.SpecialResource.Name, "", nil, "", ""); err != nil {
		return err
	}

	return nil
}

// ReconcileChart Reconcile Hardware Configurations
func (r *SpecialResourceReconciler) ReconcileChart(ctx context.Context, wi *WorkItem) error {
	// Leave this here, this is crucial for all following work
	// Creating and setting the working namespace for the specialresource
	// specialresource name == namespace if not metadata.namespace is set
	if err := r.createSpecialResourceNamespace(ctx, wi); err != nil {
		return fmt.Errorf("could not create the SpecialResource's namespace: %w", err)
	}

	if err := r.createImagePullerRoleBinding(ctx, wi); err != nil {
		return fmt.Errorf("could not create ImagePuller RoleBinding: %w", err)
	}

	if err := r.ReconcileChartStates(ctx, wi); err != nil {
		return fmt.Errorf("cannot reconcile hardware states: %w", err)
	}

	return nil
}
