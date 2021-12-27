package controllers

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/slice"
	"github.com/openshift-psap/special-resource-operator/pkg/state"
	"github.com/openshift-psap/special-resource-operator/pkg/upgrade"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

func createImagePullerRoleBinding(r *SpecialResourceReconciler) error {

	if found := slice.Contains(r.dependency.Tags, "image-puller"); !found {
		log.Info("dep", "ImagePuller", found)
	}

	log.Info("Looking for ImagePuller RoleBinding")
	rb := &unstructured.Unstructured{}
	rb.SetAPIVersion("rbac.authorization.k8s.io/v1")
	rb.SetKind("RoleBinding")

	namespacedName := types.NamespacedName{Namespace: r.specialresource.Spec.Namespace, Name: "system:image-pullers"}
	err := clients.Interface.Get(context.TODO(), namespacedName, rb)
	if apierrors.IsNotFound(err) {
		log.Info("Warning: RoleBinding system:image-pullers not found. Can be ignored on vanilla k8s or when namespace is being created.")
		return nil
	} else if err != nil {
		return errors.Wrap(err, "Error checking for image-pullers roleBinding")
	}

	newSubject := make(map[string]interface{})
	newSubjects := make([]interface{}, 0)

	newSubject["kind"] = "ServiceAccount"
	newSubject["name"] = "builder"
	newSubject["namespace"] = r.parent.Spec.Namespace

	if apierrors.IsNotFound(err) {

		log.Info("ImagePuller RoleBinding not found, creating")
		rb.SetName("system:image-puller")
		rb.SetNamespace(r.specialresource.Spec.Namespace)

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

		if err = clients.Interface.Create(context.TODO(), rb); err != nil {
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

	log.Info("ImageReference RoleBinding found, updating")

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

			if namespace == r.parent.Spec.Namespace {
				log.Info("ImageReference ServiceAccount found, returning")
				return nil
			}
		default:
			log.Info("subject", "DEFAULT NOT THE CORRECT TYPE", subject)
		}
	}

	oldSubjects = append(oldSubjects, newSubject)

	if err = unstructured.SetNestedSlice(rb.Object, oldSubjects, "subjects"); err != nil {
		return err
	}

	if err = clients.Interface.Update(context.TODO(), rb); err != nil {
		return fmt.Errorf("couldn't Update Resource: %w", err)
	}

	return nil
}

// ReconcileChartStates Reconcile Hardware States
func ReconcileChartStates(r *SpecialResourceReconciler, templates *unstructured.Unstructured) error {

	nostate := r.chart
	nostate.Templates = []*chart.File{}

	stateYAMLS := []*chart.File{}

	// First get all non-state related files from the templates
	// and save the states in a temporary slice for single execution
	for _, template := range r.chart.Templates {
		if r.Assets.ValidStateName(template.Name) {
			stateYAMLS = append(stateYAMLS, template)
		} else {
			nostate.Templates = append(nostate.Templates, template)
		}
	}

	sort.Slice(stateYAMLS, func(i, j int) bool {
		return stateYAMLS[i].Name < stateYAMLS[j].Name
	})

	for _, stateYAML := range stateYAMLS {

		log.Info("Executing", "State", stateYAML.Name)

		if r.specialresource.Spec.Debug {
			fmt.Printf("STATE YAML --------------------------------------------------\n%s\n\n", stateYAML.Data)
		}

		// Every YAML is one state, we generate the name of the
		// state special-resource + first 4 digits of the state
		// e.g.: simple-kmod-0000 this can be used for scheduling or
		// affinity, anti-affinity
		state.GenerateName(stateYAML, r.specialresource.Name)

		step := nostate
		step.Templates = append(nostate.Templates, stateYAML)

		// We are kernel-affine if the yamlSpec uses {{.Values.kernelFullVersion}}
		// then we need to replicate the object and set a name + os + kernel version
		kernelAffine := strings.Contains(string(stateYAML.Data), ".Values.kernelFullVersion")

		var replicas int
		var version upgrade.NodeVersion

		// The cluster has more then one kernel version running
		// we're replicating the driver-container DaemonSet to
		// the number of kernel versions running in the cluster
		if len(RunInfo.ClusterUpgradeInfo) == 0 {
			return errors.New("no KernelVersion detected, something is wrong")
		}

		//var replicas is to keep track of the number of replicas
		// and either to break or continue the for looop
		for RunInfo.KernelFullVersion, version = range RunInfo.ClusterUpgradeInfo {

			RunInfo.ClusterVersionMajorMinor = version.ClusterVersion
			RunInfo.OperatingSystemDecimal = version.OSVersion
			RunInfo.OperatingSystemMajorMinor = version.OSMajorMinor
			RunInfo.OperatingSystemMajor = version.OSMajor
			RunInfo.DriverToolkitImage = version.DriverToolkit.ImageURL

			if kernelAffine {
				log.Info("KernelAffine: ClusterUpgradeInfo",
					"kernel", RunInfo.KernelFullVersion,
					"os", RunInfo.OperatingSystemDecimal,
					"cluster", RunInfo.ClusterVersionMajorMinor,
					"driverToolkitImage", RunInfo.DriverToolkitImage)
			}

			var err error

			step.Values, err = chartutil.CoalesceValues(&step, r.values.Object)
			if err != nil {
				return err
			}

			rinfo, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&RunInfo)
			if err != nil {
				return err
			}

			step.Values, err = chartutil.CoalesceValues(&step, rinfo)
			if err != nil {
				return err
			}

			if r.specialresource.Spec.Debug {
				d, _ := yaml.Marshal(step.Values)
				fmt.Printf("STEP VALUES --------------------------------------------------\n%s\n\n", d)
			}

			err = r.Helmer.Run(step, step.Values,
				&r.specialresource,
				r.specialresource.Name,
				r.specialresource.Spec.Namespace,
				r.specialresource.Spec.NodeSelector,
				RunInfo.KernelFullVersion,
				RunInfo.OperatingSystemDecimal,
				r.specialresource.Spec.Debug)
			//if err != nil {
			//	return err
			//}

			replicas += 1

			// If the first replica fails we want to create all remaining
			// ones for parallel startup, otherwise we would wait for the first
			// then for the second etc.
			if err != nil && replicas == len(RunInfo.ClusterUpgradeInfo) {
				r.Metrics.SetCompletedState(r.specialresource.Name, stateYAML.Name, 0)
				return fmt.Errorf("failed to create state %s: %w ", stateYAML.Name, err)
			}

			// We're always doing one run to create a non kernel affine resource
			if !kernelAffine {
				break
			}
		}

		r.Metrics.SetCompletedState(r.specialresource.Name, stateYAML.Name, 1)
		// If resource available, label the nodes according to the current state
		// if e.g driver-container ready -> specialresource.openshift.io/driver-container:ready
		operatorStatusUpdate(&r.specialresource, state.CurrentName)

		if err := labelNodesAccordingToState(r.specialresource.Spec.NodeSelector); err != nil {
			return err
		}
	}

	// We're done with states now execute the part of the chart without
	// states we need to reconcile the nostate Chart
	var err error
	nostate.Values, err = chartutil.CoalesceValues(&nostate, r.values.Object)
	if err != nil {
		return err
	}

	rinfo, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&RunInfo)
	if err != nil {
		return err
	}

	nostate.Values, err = chartutil.CoalesceValues(&nostate, rinfo)
	if err != nil {
		return err
	}

	return r.Helmer.Run(nostate, nostate.Values,
		&r.specialresource,
		r.specialresource.Name,
		r.specialresource.Spec.Namespace,
		r.specialresource.Spec.NodeSelector,
		RunInfo.KernelFullVersion,
		RunInfo.OperatingSystemDecimal,
		false)
}

func createSpecialResourceNamespace(r *SpecialResourceReconciler) error {

	ns := []byte(`apiVersion: v1
kind: Namespace
metadata:
  annotations:
    specialresource.openshift.io/wait: "true"
    openshift.io/cluster-monitoring: "true"
  name: `)

	if r.specialresource.Spec.Namespace != "" {
		add := []byte(r.specialresource.Spec.Namespace)
		ns = append(ns, add...)
	} else {
		r.specialresource.Spec.Namespace = r.specialresource.Name
		add := []byte(r.specialresource.Spec.Namespace)
		ns = append(ns, add...)
	}

	if err := r.Creator.CreateFromYAML(ns, false, &r.specialresource, r.specialresource.Name, "", nil, "", ""); err != nil {
		log.Info("Cannot reconcile specialresource namespace, something went horribly wrong")
		return err
	}

	return nil
}

// ReconcileChart Reconcile Hardware Configurations
func ReconcileChart(r *SpecialResourceReconciler) error {

	var templates *unstructured.Unstructured

	// Leave this here, this is crucial for all following work
	// Creating and setting the working namespace for the specialresource
	// specialresource name == namespace if not metadata.namespace is set
	if err := createSpecialResourceNamespace(r); err != nil {
		return fmt.Errorf("could not create the SpecialResource's namespace: %w", err)
	}

	if err := createImagePullerRoleBinding(r); err != nil {
		return fmt.Errorf("could not create ImagePuller RoleBinding: %w", err)
	}

	if err := ReconcileChartStates(r, templates); err != nil {
		return fmt.Errorf("cannot reconcile hardware states: %w", err)
	}

	return nil
}
