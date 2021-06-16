package controllers

import (
	"context"
	"sort"
	"strings"

	"github.com/go-logr/logr"
	"github.com/openshift-psap/special-resource-operator/pkg/assets"
	"github.com/openshift-psap/special-resource-operator/pkg/cache"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/exit"
	"github.com/openshift-psap/special-resource-operator/pkg/filter"
	"github.com/openshift-psap/special-resource-operator/pkg/hash"
	"github.com/openshift-psap/special-resource-operator/pkg/helmer"
	"github.com/openshift-psap/special-resource-operator/pkg/kernel"
	"github.com/openshift-psap/special-resource-operator/pkg/metrics"
	"github.com/openshift-psap/special-resource-operator/pkg/resource"
	"github.com/openshift-psap/special-resource-operator/pkg/slice"
	"github.com/openshift-psap/special-resource-operator/pkg/state"
	"github.com/openshift-psap/special-resource-operator/pkg/upgrade"
	"github.com/openshift-psap/special-resource-operator/pkg/warn"
	"github.com/openshift-psap/special-resource-operator/pkg/yamlutil"
	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"
)

func getChartTemplates(r *SpecialResourceReconciler) (*unstructured.Unstructured, error) {

	log.Info("Looking for chart templates ConfigMap for")
	cm := &unstructured.Unstructured{}
	cm.SetAPIVersion("v1")
	cm.SetKind("ConfigMap")

	namespacedName := types.NamespacedName{Namespace: r.specialresource.Spec.Namespace, Name: r.specialresource.Name}
	err := clients.Interface.Get(context.TODO(), namespacedName, cm)

	if apierrors.IsNotFound(err) {
		log.Info("SpecialResource chart templates ConfigMap not found, using local repository \"/charts\" for")
		return nil, nil
	}
	return cm, nil
}

func createImagePullerRoleBinding(r *SpecialResourceReconciler) error {

	if found := slice.Contains(r.dependency.Tags, "image-puller"); !found {
		log.Info("dep", "ImagePuller", found)
	}

	log.Info("Looking for ImagePuller RoleBinding")
	rb := &unstructured.Unstructured{}
	rb.SetAPIVersion("rbac.authorization.k8s.io/v1")
	rb.SetKind("RoleBinding")

	namespacedName := types.NamespacedName{Namespace: r.specialresource.Spec.Namespace, Name: "system:image-puller"}
	err := clients.Interface.Get(context.TODO(), namespacedName, rb)

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

			log.Info("ImageReference", "namespace", namespace)
			log.Info("ImageReference", "r.namespace", r.parent.Spec.Namespace)

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

	// If we have found a ConfigMap with the templates use this to populate
	// the states otherwise use the templates from the chart directory
	if templates != nil {
		log.Info("Getting states from ConfigMap")
		stateYAMLS = assets.FromConfigMap(templates)
	}

	sort.Slice(stateYAMLS, func(i, j int) bool {
		return stateYAMLS[i].Name < stateYAMLS[j].Name
	})

	for _, stateYAML := range stateYAMLS {

		log.Info("Executing", "State", stateYAML.Name)

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
			RunInfo.DriverToolkitImage = version.DriverToolkit.ImageURL

			if kernelAffine {
				log.Info("KernelAffine: ClusterUpgradeInfo",
					"kernel", RunInfo.KernelFullVersion,
					"os", RunInfo.OperatingSystemDecimal,
					"cluster", RunInfo.ClusterVersionMajorMinor,
					"driverToolkit", RunInfo.DriverToolkitImage)
			}

			var err error
			step.Values, err = chartutil.CoalesceValues(&step, r.specialresource.Spec.Set.Object)
			exit.OnError(err)

			rinfo, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&RunInfo)
			exit.OnError(err)

			step.Values, err = chartutil.CoalesceValues(&step, rinfo)
			exit.OnError(err)

			/* DO NOT REMOVE: Used for debuggging
			d, _ := yaml.Marshal(step.Values)
			fmt.Printf("%s\n", d)
			*/

			yaml, err := helmer.TemplateChart(step, step.Values)
			exit.OnError(err)

			// DO NOT REMOVE: fmt.Printf("--------------------------------------------------\n\n%s\n\n", yaml)
			err = createFromYAML(yaml, r, r.specialresource.Spec.Namespace,
				RunInfo.KernelFullVersion,
				RunInfo.OperatingSystemDecimal)

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
	}

	// We're done with states now execute the part of the chart without
	// states we need to reconcile the nostate Chart
	var err error
	nostate.Values, err = chartutil.CoalesceValues(&nostate, r.specialresource.Spec.Set.Object)
	exit.OnError(err)

	rinfo, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&RunInfo)
	exit.OnError(err)

	nostate.Values, err = chartutil.CoalesceValues(&nostate, rinfo)
	exit.OnError(err)

	yaml, err := helmer.TemplateChart(nostate, nostate.Values)
	exit.OnError(err)

	// If we only have SRO states, the nostate may be empty, just return
	if len(yaml) <= 1 {
		log.Info("NoState chart empty, returning")
		return nil
	}

	if err := createFromYAML(yaml, r, r.specialresource.Spec.Namespace,
		RunInfo.KernelFullVersion,
		RunInfo.OperatingSystemDecimal); err != nil {
		return errors.Wrap(err, "Failed to create nostate chart: "+nostate.Name())
	}

	return nil
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
	if err := createFromYAML(ns, r, "", "", ""); err != nil {
		log.Info("Cannot reconcile specialresource namespace, something went horribly wrong")
		exit.OnError(err)
	}
}

// ReconcileChart Reconcile Hardware Configurations
func ReconcileChart(r *SpecialResourceReconciler) error {

	var err error
	var templates *unstructured.Unstructured

	// Leave this here, this is crucial for all following work
	// Creating and setting the working namespace for the specialresource
	// specialresource name == namespace if not metadata.namespace is set
	createSpecialResourceNamespace(r)
	if err := createImagePullerRoleBinding(r); err != nil {
		return errors.Wrap(err, "Could not create ImagePuller RoleBinding ")

	}

	// Check if we have a ConfigMap deployed in the specialresrouce
	// namespace if not fallback to the local repository.
	// ConfigMap can be used to overrride the local repository templates
	// for testing.
	log.Info("Getting chart templates from ConfigMap")
	if templates, err = getChartTemplates(r); err != nil {
		return errors.Wrap(err, "Cannot get ConfigMap with chart templates")
	}

	err = cache.Nodes(r.specialresource.Spec.NodeSelector, false)
	exit.OnError(errors.Wrap(err, "Failed to cache Nodes"))

	getRuntimeInformation(r)
	logRuntimeInformation()

	if err := ReconcileChartStates(r, templates); err != nil {
		return errors.Wrap(err, "Cannot reconcile hardware states")
	}

	return nil
}

func createFromYAML(yamlFile []byte, r *SpecialResourceReconciler,
	namespace string,
	kernelFullVersion string,
	operatingSystemMajorMinor string) error {

	scanner := yamlutil.NewYAMLScanner(yamlFile)

	nodeSelector := r.specialresource.Spec.NodeSelector

	for scanner.Scan() {

		yamlSpec := scanner.Bytes()

		obj := &unstructured.Unstructured{}
		jsonSpec, err := yaml.YAMLToJSON(yamlSpec)
		if err != nil {
			return errors.Wrap(err, "Could not convert yaml file to json"+string(yamlSpec))
		}

		err = obj.UnmarshalJSON(jsonSpec)
		exit.OnError(errors.Wrap(err, "Cannot unmarshall json spec, check your manifest: "+string(jsonSpec)))

		if resource.IsNamespaced(obj.GetKind()) {
			obj.SetNamespace(namespace)
		}

		// We used this for predicate filtering, we're watching a lot of
		// API Objects we want to ignore all objects that do not have this
		// label.
		filter.SetLabel(obj)

		// kernel affinity related attributes only set if there is an
		// annotation specialresource.openshift.io/kernel-affine: true
		if kernel.IsObjectAffine(obj) {
			err := kernel.SetAffineAttributes(obj, kernelFullVersion,
				operatingSystemMajorMinor)
			exit.OnError(errors.Wrap(err, "Cannot set kernel affine attributes"))
		}

		// Add nodeSelector terms for the specialresource
		// we do not want to spread HW enablement stacks on all nodes
		err = resource.SetNodeSelectorTerms(obj, nodeSelector)
		exit.OnError(errors.Wrap(err, "setting NodeSelectorTerms failed"))

		// We are only building a driver-container if we cannot pull the image
		// We are asuming that vendors provide pre compiled DriverContainers
		// If err == nil, build a new container, if err != nil skip it
		if err := rebuildDriverContainer(obj, r); err != nil {
			log.Info("Skipping building driver-container", "Name", obj.GetName())
			return nil
		}

		// Callbacks before CRUD will update the manifests
		if err := beforeCRUDhooks(obj, r); err != nil {
			return errors.Wrap(err, "Before CRUD hooks failed")
		}
		// Create Update Delete Patch resources
		err = CRUD(obj, r)
		exit.OnError(errors.Wrap(err, "CRUD exited non-zero"))

		// Callbacks after CRUD will wait for ressource and check status
		if err := afterCRUDhooks(obj, r); err != nil {
			return errors.Wrap(err, "After CRUD hooks failed")
		}

	}

	if err := scanner.Err(); err != nil {
		return errors.Wrap(err, "Failed to scan manifest")
	}
	return nil
}

// CRUD Create Update Delete Resource
func CRUD(obj *unstructured.Unstructured, r *SpecialResourceReconciler) error {

	var logger logr.Logger
	if resource.IsNamespaced(obj.GetKind()) {
		logger = log.WithValues("Kind", obj.GetKind()+": "+obj.GetNamespace()+"/"+obj.GetName())
	} else {
		logger = log.WithValues("Kind", obj.GetKind()+": "+obj.GetName())
	}

	found := obj.DeepCopy()

	// SpecialResource is the parent, all other objects are childs and need a reference
	// but only set the ownerreference if created by SRO do not set ownerreference per default
	if obj.GetKind() != "SpecialResource" {
		err := controllerutil.SetControllerReference(&r.specialresource, obj, r.Scheme)
		warn.OnError(err)
	}

	err := clients.Interface.Get(context.TODO(), types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, found)

	if apierrors.IsNotFound(err) {
		logger.Info("Not found, creating")

		hash.Annotate(obj)

		if err := clients.Interface.Create(context.TODO(), obj); err != nil {
			return errors.Wrap(err, "Couldn't create resource")
		}

		return nil
	}

	if apierrors.IsForbidden(err) {
		return errors.Wrap(err, "Forbidden check Role, ClusterRole and Bindings for operator")
	}

	if err != nil {
		return errors.Wrap(err, "Unexpected error")
	}

	// Not updating Pod because we can only update image and some other
	// specific minor fields.
	if resource.IsNotUpdateable(obj.GetKind()) {
		log.Info("Not Updateable", "Resource", obj.GetKind())
		return nil
	}

	if equality.Semantic.DeepEqual(found, obj) {
		log.Info("equality.Semantic, equal")
	}

	if hash.AnnotationEqual(found, obj) {
		log.Info("Found, not updating, hash the same: " + found.GetKind() + "/" + found.GetName())
		return nil
	}

	logger.Info("Found, updating")
	required := obj.DeepCopy()

	hash.Annotate(required)

	// required.ResourceVersion = found.ResourceVersion this is only needed
	// before we update a resource, we do not care when creating, hence
	// !leave this here!
	if err := resource.UpdateResourceVersion(required, found); err != nil {
		return errors.Wrap(err, "Couldn't Update ResourceVersion")
	}

	if err := clients.Interface.Update(context.TODO(), required); err != nil {
		return errors.Wrap(err, "Couldn't Update Resource")
	}

	return nil
}

func rebuildDriverContainer(obj *unstructured.Unstructured, r *SpecialResourceReconciler) error {

	logger := log.WithValues("Kind", obj.GetKind(), "Namespace", obj.GetNamespace(), "Name", obj.GetName())
	// BuildConfig are currently not triggered by an update need to delete first
	if obj.GetKind() == "BuildConfig" {
		annotations := obj.GetAnnotations()
		if vendor, ok := annotations["specialresource.openshift.io/driver-container-vendor"]; ok {
			logger.Info("driver-container-vendor", "vendor", vendor)
			if vendor == RunInfo.UpdateVendor {
				logger.Info("vendor == updateVendor", "vendor", vendor, "updateVendor", RunInfo.UpdateVendor)
				return nil
			}
			logger.Info("vendor != updateVendor", "vendor", vendor, "updateVendor", RunInfo.UpdateVendor)
			return errors.New("vendor != updateVendor")
		}
		logger.Info("No annotation driver-container-vendor found")
		return errors.New("No driver-container-vendor found, nor vendor == updateVendor")
	}

	return nil
}
