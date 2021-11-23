package controllers

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/openshift-psap/special-resource-operator/pkg/assets"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/exit"
	"github.com/openshift-psap/special-resource-operator/pkg/helmer"
	"github.com/openshift-psap/special-resource-operator/pkg/metrics"
	"github.com/openshift-psap/special-resource-operator/pkg/resource"
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
		err := unstructured.SetNestedField(rb.Object, "rbac.authorization.k8s.io", "roleRef", "apiGroup")
		exit.OnError(err)
		err = unstructured.SetNestedField(rb.Object, "ClusterRole", "roleRef", "kind")
		exit.OnError(err)
		err = unstructured.SetNestedField(rb.Object, "system:image-puller", "roleRef", "name")
		exit.OnError(err)

		newSubjects = append(newSubjects, newSubject)

		err = unstructured.SetNestedSlice(rb.Object, newSubjects, "subjects")
		exit.OnError(err)

		if err := clients.Interface.Create(context.TODO(), rb); err != nil {
			return errors.Wrap(err, "Couldn't Create Resource")
		}

		return nil
	}

	if apierrors.IsForbidden(err) {
		return errors.Wrap(err, "Forbidden check Role, ClusterRole and Bindings for operator")
	}

	if err != nil {
		return errors.Wrap(err, "Unexpected error")
	}

	log.Info("ImageReference RoleBinding found, updating")

	oldSubjects, _, err := unstructured.NestedSlice(rb.Object, "subjects")
	exit.OnError(err)

	for _, subject := range oldSubjects {
		switch subject := subject.(type) {
		case map[string]interface{}:
			namespace, _, err := unstructured.NestedString(subject, "namespace")
			exit.OnError(err)

			if namespace == r.parent.Spec.Namespace {
				log.Info("ImageReference ServiceAccount found, returning")
				return nil
			}
		default:
			log.Info("subject", "DEFAULT NOT THE CORRECT TYPE", subject)
		}
	}

	oldSubjects = append(oldSubjects, newSubject)

	err = unstructured.SetNestedSlice(rb.Object, oldSubjects, "subjects")
	exit.OnError(err)

	if err := clients.Interface.Update(context.TODO(), rb); err != nil {
		return errors.Wrap(err, "Couldn't Update Resource")
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
		if assets.ValidStateName(template.Name) {
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
			exit.OnError(errors.New("No KernelVersion detected, something is wrong"))
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
			exit.OnError(err)

			rinfo, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&RunInfo)
			exit.OnError(err)

			step.Values, err = chartutil.CoalesceValues(&step, rinfo)
			exit.OnError(err)

			if r.specialresource.Spec.Debug {
				d, _ := yaml.Marshal(step.Values)
				fmt.Printf("STEP VALUES --------------------------------------------------\n%s\n\n", d)
			}

			err = helmer.Run(step, step.Values,
				&r.specialresource,
				r.specialresource.Name,
				r.specialresource.Spec.Namespace,
				r.specialresource.Spec.NodeSelector,
				RunInfo.KernelFullVersion,
				RunInfo.OperatingSystemDecimal,
				r.specialresource.Spec.Debug)
			//exit.OnError(err)

			replicas += 1

			// If the first replica fails we want to create all remaining
			// ones for parallel startup, otherwise we would wait for the first
			// then for the second etc.
			if err != nil && replicas == len(RunInfo.ClusterUpgradeInfo) {
				metrics.SetCompletedState(r.specialresource.Name, stateYAML.Name, 0)
				return errors.Wrap(err, "Failed to create state: "+stateYAML.Name)
			}

			// We're always doing one run to create a non kernel affine resource
			if !kernelAffine {
				break
			}

		}

		metrics.SetCompletedState(r.specialresource.Name, stateYAML.Name, 1)
		// If resource available, label the nodes according to the current state
		// if e.g driver-container ready -> specialresource.openshift.io/driver-container:ready
		operatorStatusUpdate(&r.specialresource, state.CurrentName)
		err := labelNodesAccordingToState(r.specialresource.Spec.NodeSelector)
		exit.OnError(err)

	}

	// We're done with states now execute the part of the chart without
	// states we need to reconcile the nostate Chart
	var err error
	nostate.Values, err = chartutil.CoalesceValues(&nostate, r.values.Object)
	exit.OnError(err)

	rinfo, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&RunInfo)
	exit.OnError(err)

	nostate.Values, err = chartutil.CoalesceValues(&nostate, rinfo)
	exit.OnError(err)

	return helmer.Run(nostate, nostate.Values,
		&r.specialresource,
		r.specialresource.Name,
		r.specialresource.Spec.Namespace,
		r.specialresource.Spec.NodeSelector,
		RunInfo.KernelFullVersion,
		RunInfo.OperatingSystemDecimal,
		false)
}

func createSpecialResourceNamespace(r *SpecialResourceReconciler) {

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
	if err := resource.CreateFromYAML(ns, false, &r.specialresource, r.specialresource.Name, "", nil, "", ""); err != nil {
		log.Info("Cannot reconcile specialresource namespace, something went horribly wrong")
		exit.OnError(err)
	}
}

// ReconcileChart Reconcile Hardware Configurations
func ReconcileChart(r *SpecialResourceReconciler) error {

	var templates *unstructured.Unstructured

	// Leave this here, this is crucial for all following work
	// Creating and setting the working namespace for the specialresource
	// specialresource name == namespace if not metadata.namespace is set
	createSpecialResourceNamespace(r)
	if err := createImagePullerRoleBinding(r); err != nil {
		return errors.Wrap(err, "Could not create ImagePuller RoleBinding")
	}

	if err := ReconcileChartStates(r, templates); err != nil {
		return errors.Wrap(err, "Cannot reconcile hardware states")
	}

	return nil
}
