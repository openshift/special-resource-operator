package controllers

import (
	"bytes"
	"context"
	"html/template"
	"sort"
	"strings"

	"github.com/go-logr/logr"
	"github.com/openshift-psap/special-resource-operator/pkg/assets"
	"github.com/openshift-psap/special-resource-operator/pkg/exit"
	"github.com/openshift-psap/special-resource-operator/pkg/hash"
	"github.com/openshift-psap/special-resource-operator/pkg/metrics"
	"github.com/openshift-psap/special-resource-operator/pkg/yamlutil"
	errs "github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"
)

func getHardwareConfiguration(r *SpecialResourceReconciler) (*unstructured.Unstructured, error) {

	log.Info("Looking for Hardware Configuration ConfigMap for")
	cm := &unstructured.Unstructured{}
	cm.SetAPIVersion("v1")
	cm.SetKind("ConfigMap")

	namespacedName := types.NamespacedName{Namespace: r.specialresource.Spec.Namespace, Name: r.specialresource.Name}
	err := r.Get(context.TODO(), namespacedName, cm)

	if apierrors.IsNotFound(err) {
		log.Info("Hardware Configuration ConfigMap not found, creating from local repository (/opt/sro/recipes) for")
		manifests := "/opt/sro/recipes/" + r.specialresource.Name + "/manifests"
		return getLocalHardwareConfiguration(manifests, r.specialresource.Name)
	}

	return cm, nil
}

func getLocalHardwareConfiguration(path string, specialresource string) (*unstructured.Unstructured, error) {

	cm := &unstructured.Unstructured{}
	cm.SetAPIVersion("v1")
	cm.SetKind("ConfigMap")
	cm.SetName(specialresource)

	manifests := assets.GetFrom(path)

	data := map[string]string{}

	for _, manifest := range manifests {
		data[string(manifest.Name)] = string(manifest.Content)
	}

	if err := unstructured.SetNestedStringMap(cm.Object, data, "data"); err != nil {
		return cm, errs.Wrap(err, "Couldn't update ConfigMap data field")
	}

	return cm, nil
}

func createImagePullerRoleBinding(r *SpecialResourceReconciler) error {

	log.Info("dep", "ImageReference", r.dependency.ImageReference)
	log.Info("dep", "Name", r.dependency.Name)

	if r.dependency.ImageReference != "true" {
		return nil
	}

	log.Info("Looking for ImageReference RoleBinding")
	rb := &unstructured.Unstructured{}
	rb.SetAPIVersion("rbac.authorization.k8s.io/v1")
	rb.SetKind("RoleBinding")

	namespacedName := types.NamespacedName{Namespace: r.specialresource.Spec.Namespace, Name: "system:image-puller"}
	err := r.Get(context.TODO(), namespacedName, rb)

	newSubject := make(map[string]interface{})
	newSubjects := make([]interface{}, 0)

	newSubject["kind"] = "ServiceAccount"
	newSubject["name"] = "builder"
	newSubject["namespace"] = r.parent.Spec.Namespace

	if apierrors.IsNotFound(err) {

		log.Info("ImageReference RoleBinding not found, creating")
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

		if err := r.Create(context.TODO(), rb); err != nil {
			return errs.Wrap(err, "Couldn't Create Resource")
		}

		return nil
	}

	if apierrors.IsForbidden(err) {
		return errs.Wrap(err, "Forbidden check Role, ClusterRole and Bindings for operator")
	}

	if err != nil {
		return errs.Wrap(err, "Unexpected error")
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

	if err := r.Update(context.TODO(), rb); err != nil {
		return errs.Wrap(err, "Couldn't Update Resource")
	}

	return nil
}

// ReconcileHardwareStates Reconcile Hardware States
func ReconcileHardwareStates(r *SpecialResourceReconciler, config unstructured.Unstructured) error {

	var manifests map[string]interface{}
	var err error
	var found bool

	manifests, found, err = unstructured.NestedMap(config.Object, "data")
	exit.OnErrorOrNotFound(found, err)

	states := make([]string, 0, len(manifests))
	for key := range manifests {
		states = append(states, key)
	}

	sort.Strings(states)

	for _, state := range states {

		log.Info("Executing", "State", state)
		namespacedYAML := []byte(manifests[state].(string))
		if err := createFromYAML(namespacedYAML, r, r.specialresource.Spec.Namespace); err != nil {
			metrics.SetCompletedState(r.specialresource.Name, state, 0)
			return errs.Wrap(err, "Failed to create resources")
		}
		metrics.SetCompletedState(r.specialresource.Name, state, 1)
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
	if err := createFromYAML(ns, r, ""); err != nil {
		log.Info("Cannot reconcile specialresource namespace, something went horribly wrong")
		exit.OnError(err)
	}
}

// ReconcileHardwareConfigurations Reconcile Hardware Configurations
func ReconcileHardwareConfigurations(r *SpecialResourceReconciler) error {

	var err error
	var config *unstructured.Unstructured
	// Leave this here, this is crucial for all following work
	// Creating and setting the working namespace for the specialresource
	// specialresource name == namespace if not metadata.namespace is set
	createSpecialResourceNamespace(r)
	if err := createImagePullerRoleBinding(r); err != nil {
		return errs.Wrap(err, "Could not create ImagePuller RoleBinding ")

	}

	// Check if we have a ConfigMap deployed in the specialresrouce
	// namespace if not fallback to the local repository.
	// ConfigMap can be used to overrride the local repository manifests
	// for testing.
	log.Info("Getting Configuration")
	if config, err = getHardwareConfiguration(r); err != nil {
		return errs.Wrap(err, "Error reconciling Hardware Configuration States")
	}

	log.Info("Found Hardware Configuration States", "Name", config.GetName())

	runInfo.Node.list, err = cacheNodes(r, false)
	exit.OnError(errs.Wrap(err, "Failed to cache Nodes"))

	getRuntimeInformation(r)
	logRuntimeInformation()

	if err := ReconcileHardwareStates(r, *config); err != nil {
		return errs.Wrap(err, "Cannot reconcile hardware states")
	}

	return nil
}

func templateRuntimeInformation(yamlSpec *[]byte, r runtimeInformation) error {

	spec := string(*yamlSpec)

	t := template.Must(template.New("runtime").Parse(spec))
	var buff bytes.Buffer
	if err := t.Execute(&buff, runInfo); err != nil {
		return errs.Wrap(err, "Cannot templatize spec for resource info injection, check manifest")
	}
	*yamlSpec = buff.Bytes()

	return nil
}

func setKernelAffineAttributes(obj *unstructured.Unstructured, kernelAffinity bool) error {

	if kernelAffinity {
		kernelVersion := strings.ReplaceAll(runInfo.KernelFullVersion, "_", "-")
		hash64 := hash.FNV64a(runInfo.OperatingSystemMajorMinor + "-" + kernelVersion)
		name := obj.GetName() + "-" + hash64
		obj.SetName(name)

		if obj.GetKind() == "DaemonSet" {
			err := unstructured.SetNestedField(obj.Object, name, "metadata", "labels", "app")
			exit.OnError(err)
			err = unstructured.SetNestedField(obj.Object, name, "spec", "selector", "matchLabels", "app")
			exit.OnError(err)
			err = unstructured.SetNestedField(obj.Object, name, "spec", "template", "metadata", "labels", "app")
			exit.OnError(err)
			err = unstructured.SetNestedField(obj.Object, name, "spec", "template", "metadata", "labels", "app")
			exit.OnError(err)
		}

		if err := setKernelVersionNodeAffinity(obj); err != nil {
			return errs.Wrap(err, "Cannot set kernel version node affinity for obj: "+obj.GetKind())
		}
	}
	return nil
}

func createFromYAML(yamlFile []byte, r *SpecialResourceReconciler, namespace string) error {

	scanner := yamlutil.NewYAMLScanner(yamlFile)

	for scanner.Scan() {

		yamlSpec := scanner.Bytes()

		// We are kernel-affine if the yamlSpec uses {{.KernelFullVersion}}
		// then we need to replicate the object and set a name + os + kernel version
		kernelAffinity := strings.Contains(string(yamlSpec), "{{.KernelFullVersion}}")

		var version nodeUpgradeVersion
		var replicas [][]byte // This is to keep track of the number of replicas
		// and either to break or continue the for looop
		for runInfo.KernelFullVersion, version = range runInfo.ClusterUpgradeInfo {

			runInfo.ClusterVersionMajorMinor = version.clusterVersion
			runInfo.OperatingSystemDecimal = version.rhelVersion

			if kernelAffinity {
				log.Info("ClusterUpgradeInfo",
					"kernel", runInfo.KernelFullVersion,
					"rhel", runInfo.OperatingSystemDecimal,
					"cluster", runInfo.ClusterVersionMajorMinor)
			}

			replicas = append(replicas, yamlSpec)

			// We can pass template information from the CR to the yamls
			// thats why we are running 2 passes.
			if err := templateRuntimeInformation(&replicas[len(replicas)-1], runInfo); err != nil {
				return errs.Wrap(err, "Cannot inject runtime information 1st pass")
			}

			if err := templateRuntimeInformation(&replicas[len(replicas)-1], runInfo); err != nil {
				return errs.Wrap(err, "Cannot inject runtime information 2nd pass")
			}

			obj := &unstructured.Unstructured{}
			jsonSpec, err := yaml.YAMLToJSON(replicas[len(replicas)-1])
			if err != nil {
				return errs.Wrap(err, "Could not convert yaml file to json"+string(yamlSpec))
			}

			err = obj.UnmarshalJSON(jsonSpec)
			exit.OnError(errs.Wrap(err, "Cannot unmarshall json spec, check your manifests"))

			if resourceIsNamespaced(obj.GetKind()) {
				obj.SetNamespace(namespace)
			}

			// kernel affinity related attributes
			err = setKernelAffineAttributes(obj, kernelAffinity)
			exit.OnError(errs.Wrap(err, "Cannot set kernel affine attributes"))

			// We are only building a driver-container if we cannot pull the image
			// We are assuming that vendors provide pre-compiled DriverContainers
			// If err == nil, build a new container, if err != nil skip it
			if err := rebuildDriverContainer(obj, r); err != nil {
				log.Info("Skipping building driver-container", "Name", obj.GetName())
				return nil
			}

			// The cluster has more then one kernel version running
			// we're replicating the driver-container DaemonSet to
			// the number of kernel versions running in the cluster
			if len(runInfo.ClusterUpgradeInfo) == 0 {
				exit.OnError(errs.New("No KernelVersion detected, something is wrong"))
			}

			// Callbacks before CRUD will update the manifests
			if err := beforeCRUDhooks(obj, r); err != nil {
				return errs.Wrap(err, "Before CRUD hooks failed")
			}
			// Create Update Delete Patch resources
			err = CRUD(obj, r)
			exit.OnError(errs.Wrap(err, "CRUD exited non-zero"))

			// Callbacks after CRUD will wait for resource and check status
			// Only return if we have created all replicas otherwise
			// we will reconcile only the first replica
			if err := afterCRUDhooks(obj, r); err != nil && len(replicas) == len(runInfo.ClusterUpgradeInfo) {
				return errs.Wrap(err, "After CRUD hooks failed")
			}

			if !kernelAffinity {
				break
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return errs.Wrap(err, "Failed to scan manifest")
	}
	return nil
}

// Some resources need an updated resourceversion, during updates
func needToUpdateResourceVersion(kind string) bool {

	if kind == "SecurityContextConstraints" ||
		kind == "Service" ||
		kind == "ServiceMonitor" ||
		kind == "Route" ||
		kind == "BuildConfig" ||
		kind == "ImageStream" ||
		kind == "PrometheusRule" ||
		kind == "CSIDriver" ||
		kind == "Issuer" ||
		kind == "Certificate" {
		return true
	}
	return false
}

func updateResourceVersion(req *unstructured.Unstructured, found *unstructured.Unstructured) error {

	kind := found.GetKind()

	if needToUpdateResourceVersion(kind) {
		version, fnd, err := unstructured.NestedString(found.Object, "metadata", "resourceVersion")
		exit.OnErrorOrNotFound(fnd, err)

		if err := unstructured.SetNestedField(req.Object, version, "metadata", "resourceVersion"); err != nil {
			return errs.Wrap(err, "Couldn't update ResourceVersion")
		}

	}
	if kind == "Service" {
		clusterIP, fnd, err := unstructured.NestedString(found.Object, "spec", "clusterIP")
		exit.OnErrorOrNotFound(fnd, err)

		if err := unstructured.SetNestedField(req.Object, clusterIP, "spec", "clusterIP"); err != nil {
			return errs.Wrap(err, "Couldn't update clusterIP")
		}
		return nil
	}
	return nil
}

func resourceIsNamespaced(kind string) bool {
	if kind == "Namespace" ||
		kind == "ClusterRole" ||
		kind == "ClusterRoleBinding" ||
		kind == "SecurityContextConstraint" ||
		kind == "SpecialResource" {
		return false
	}
	return true
}

// CRUD Create Update Delete Resource
func CRUD(obj *unstructured.Unstructured, r *SpecialResourceReconciler) error {

	var logger logr.Logger
	if resourceIsNamespaced(obj.GetKind()) {
		logger = log.WithValues("Kind", obj.GetKind()+": "+obj.GetNamespace()+"/"+obj.GetName())
	} else {
		logger = log.WithValues("Kind", obj.GetKind()+": "+obj.GetName())
	}

	found := obj.DeepCopy()

	// SpecialResource is the parent, all other objects are childs and need a reference
	if obj.GetKind() != "SpecialResource" {
		if err := controllerutil.SetControllerReference(&r.specialresource, obj, r.Scheme); err != nil {
			return errs.Wrap(err, "Failed to set controller reference")
		}
	}

	err := r.Get(context.TODO(), types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, found)

	if apierrors.IsNotFound(err) {
		logger.Info("Not found, creating")
		if err := r.Create(context.TODO(), obj); err != nil {
			return errs.Wrap(err, "Couldn't Create Resource")
		}
		return nil
	}

	if apierrors.IsForbidden(err) {
		return errs.Wrap(err, "Forbidden check Role, ClusterRole and Bindings for operator")
	}

	if err != nil {
		return errs.Wrap(err, "Unexpected error")
	}
	// Not updating Pod because we can only update image and some other
	// specific minor fields.
	//
	// ServiceAccounts cannot be updated, maybe delete and create?
	if obj.GetKind() == "ServiceAccount" || obj.GetKind() == "Pod" || obj.GetKind() == "BuildConfig" {
		// Not updating BuildConfig since it triggers a new build in 4.6 was not doing that in <4.6
		//logger.Info("TODO: Found, not updating, does not work, why? Secret accumulation?")
		return nil
	}

	logger.Info("Found, updating")
	required := obj.DeepCopy()

	// required.ResourceVersion = found.ResourceVersion this is only needed
	// before we update a resource, we do not care when creating, hence
	// !leave this here!
	if err := updateResourceVersion(required, found); err != nil {
		return errs.Wrap(err, "Couldn't Update ResourceVersion")
	}

	if err := r.Update(context.TODO(), required); err != nil {
		return errs.Wrap(err, "Couldn't Update Resource")
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
			if vendor == runInfo.UpdateVendor {
				logger.Info("vendor == updateVendor", "vendor", vendor, "updateVendor", runInfo.UpdateVendor)
				return nil
			}
			logger.Info("vendor != updateVendor", "vendor", vendor, "updateVendor", runInfo.UpdateVendor)
			return errs.New("vendor != updateVendor")
		}
		logger.Info("No annotation driver-container-vendor found")
		return errs.New("No driver-container-vendor found, nor vendor == updateVendor")
	}

	return nil
}
