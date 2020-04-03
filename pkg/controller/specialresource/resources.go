package specialresource

import (
	"bytes"
	"context"
	"html/template"
	"os"
	"path/filepath"
	"sort"

	errs "github.com/pkg/errors"

	monitoringV1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/openshift-psap/special-resource-operator/pkg/yamlutil"
	buildV1 "github.com/openshift/api/build/v1"
	imageV1 "github.com/openshift/api/image/v1"
	routev1 "github.com/openshift/api/route/v1"
	secv1 "github.com/openshift/api/security/v1"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"
)

type nodes struct {
	list  *unstructured.UnstructuredList
	count int64
}

var (
	manifests  = "/etc/kubernetes/special-resource/nvidia-gpu"
	kubeclient *kubernetes.Clientset
	node       = nodes{
		list:  &unstructured.UnstructuredList{},
		count: 0xDEADBEEF,
	}
)

// Add3dpartyResourcesToScheme Adds 3rd party resources To the operator
func Add3dpartyResourcesToScheme(scheme *runtime.Scheme) error {

	if err := routev1.AddToScheme(scheme); err != nil {
		return err
	}
	if err := secv1.AddToScheme(scheme); err != nil {
		return err
	}
	if err := buildV1.AddToScheme(scheme); err != nil {
		return err
	}
	if err := imageV1.AddToScheme(scheme); err != nil {
		return err
	}
	if err := monitoringV1.AddToScheme(scheme); err != nil {
		return err
	}

	return nil
}

func filePathWalkDir(root string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func cacheNodes(r *ReconcileSpecialResource, force bool) (*unstructured.UnstructuredList, error) {

	// The initial list is what we're working with
	// a SharedInformer will update the list of nodes if
	// more nodes join the cluster.
	cached := int64(len(node.list.Items))
	if cached == node.count && !force {
		return node.list, nil
	}

	node.list.SetAPIVersion("v1")
	node.list.SetKind("NodeList")

	opts := &client.ListOptions{}
	opts.SetLabelSelector(runInfo.NodeFeature + "=true")

	err := r.client.List(context.TODO(), opts, node.list)
	if err != nil {
		return nil, errors.Wrap(err, "Client cannot get NodeList")
	}

	return node.list, err
}

func getHardwareConfigurations(r *ReconcileSpecialResource) (*unstructured.UnstructuredList, error) {

	log.Info("Looking for Hardware Configuration ConfigMaps with label specialresource.openshift.io/config: true")
	cms := &unstructured.UnstructuredList{}
	cms.SetAPIVersion("v1")
	cms.SetKind("ConfigMapList")

	labels := map[string]string{"specialresource.openshift.io/config": "true"}

	opts := &client.ListOptions{}
	opts.InNamespace(r.specialresource.Namespace)
	opts.MatchingLabels(labels)

	err := r.client.List(context.TODO(), opts, cms)
	if apierrors.IsNotFound(err) {
		return nil, errs.New("Hardware Configuration ConfigMaps with label specialresource.openshift.io/config: true not found, see README and create the states")
	}

	return cms, nil
}

// ReconcileHardwareStates Reconcile Hardware States
func ReconcileHardwareStates(r *ReconcileSpecialResource, config unstructured.Unstructured) error {

	var manifests map[string]interface{}
	var err error
	var found bool

	manifests, found, err = unstructured.NestedMap(config.Object, "data")
	checkNestedFields(found, err)

	states := make([]string, 0, len(manifests))
	for key := range manifests {
		states = append(states, key)
	}

	sort.Strings(states)

	for _, state := range states {

		log.Info("Executing", "State", state)
		namespacedYAML := []byte(manifests[state].(string))
		if err := createFromYAML(namespacedYAML, r); err != nil {
			return errs.Wrap(err, "Failed to create resources")
		}
	}

	return nil
}

// ReconcileHardwareConfigurations Reconcile Hardware Configurations
func ReconcileHardwareConfigurations(r *ReconcileSpecialResource) error {

	var err error
	var configs *unstructured.UnstructuredList

	if configs, err = getHardwareConfigurations(r); err != nil {
		return errs.Wrap(err, "Error reconciling Hardware Configuration (states, Specialresource)")
	}

	for _, config := range configs.Items {

		var found bool

		annotations := config.GetAnnotations()
		log.Info("Found Hardware Configuration", "Name", config.GetName())

		short := "specialresource.openshift.io/nfd"
		if runInfo.NodeFeature, found = annotations[short]; !found || len(runInfo.NodeFeature) == 0 {
			return errs.New("ConfigMap has no " + short + " annotation cannot determine the device")
		}
		short = "specialresource.openshift.io/hardware"
		if runInfo.HardwareResource, found = annotations[short]; !found || len(runInfo.HardwareResource) == 0 {
			return errs.New("ConfigMap has no " + short + " annotation cannot determine the vendor-specialresource")
		}

		node.list, err = cacheNodes(r, false)
		exitOnError(errs.Wrap(err, "Failed to cache Nodes"))

		getRuntimeInformation(r)
		logRuntimeInformation()

		if err := ReconcileHardwareStates(r, config); err != nil {
			return errs.Wrap(err, "Cannot reconcile hardware states")
		}
	}

	return nil
}

func templateSpecialResourceInformation(yamlSpec *[]byte) error {

	spec := string(*yamlSpec)

	t := template.Must(template.New("runtime").Parse(spec))
	var buff bytes.Buffer
	if err := t.Execute(&buff, runInfo); err != nil {
		return errs.Wrap(err, "Cannot templatize spec for resource info injection, check manifest")
	}

	*yamlSpec = buff.Bytes()

	return nil
}

func createFromYAML(yamlFile []byte, r *ReconcileSpecialResource) error {

	namespace := r.specialresource.Namespace
	scanner := yamlutil.NewYAMLScanner(yamlFile)

	for scanner.Scan() {

		yamlSpec := scanner.Bytes()

		err := templateSpecialResourceInformation(&yamlSpec)
		exitOnError(errs.Wrap(err, "Cannot inject special resource information"))

		obj := &unstructured.Unstructured{}
		jsonSpec, err := yaml.YAMLToJSON(yamlSpec)
		if err != nil {
			return errs.Wrap(err, "Could not convert yaml file to json"+string(yamlSpec))
		}
		err = obj.UnmarshalJSON(jsonSpec)
		exitOnError(errs.Wrap(err, "Cannot unmarshall json spec, check your manifests"))

		// We are only building a driver-container if we cannot pull the image
		// We are asuming that vendors provide pre compiled DriverContainers
		// If err == nil, build a new container, if err != nil skip it
		if err := rebuildDriverContainer(obj, r); err != nil {
			log.Info("Skipping building driver-container", "Name", obj.GetName())
			return nil
		}

		obj.SetNamespace(namespace)

		// Callbacks before CRUD will update the manifests
		if err := beforeCRUDhooks(obj, r); err != nil {
			return errs.Wrap(err, "Before CRUD hooks failed")
		}
		// Create Update Delete Patch resources
		err = CRUD(obj, r)
		exitOnError(errs.Wrap(err, "CRUD exited non-zero"))

		// Callbacks after CRUD will wait for ressource and check status
		if err := afterCRUDhooks(obj, r); err != nil {
			return errs.Wrap(err, "After CRUD hooks failed")
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
		kind == "PrometheusRule" {
		return true
	}
	return false
}

func updateResourceVersion(req *unstructured.Unstructured, found *unstructured.Unstructured) error {

	kind := found.GetKind()

	if needToUpdateResourceVersion(kind) {
		version, fnd, err := unstructured.NestedString(found.Object, "metadata", "resourceVersion")
		checkNestedFields(fnd, err)

		if err := unstructured.SetNestedField(req.Object, version, "metadata", "resourceVersion"); err != nil {
			return errs.Wrap(err, "Couldn't update ResourceVersion")
		}

	}
	if kind == "Service" {
		clusterIP, fnd, err := unstructured.NestedString(found.Object, "spec", "clusterIP")
		checkNestedFields(fnd, err)

		if err := unstructured.SetNestedField(req.Object, clusterIP, "spec", "clusterIP"); err != nil {
			return errs.Wrap(err, "Couldn't update clusterIP")
		}
		return nil
	}
	return nil
}

// CRUD Create Update Delete Resource
func CRUD(obj *unstructured.Unstructured, r *ReconcileSpecialResource) error {

	logger := log.WithValues("Kind", obj.GetKind(), "Namespace", obj.GetNamespace(), "Name", obj.GetName())
	found := obj.DeepCopy()

	if err := controllerutil.SetControllerReference(r.specialresource, obj, r.scheme); err != nil {
		return errs.Wrap(err, "Failed to set controller reference")
	}

	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, found)

	if apierrors.IsNotFound(err) {
		logger.Info("Not found, creating")
		if err := r.client.Create(context.TODO(), obj); err != nil {
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

	// ServiceAccounts cannot be updated, maybe delete and create?
	if obj.GetKind() == "ServiceAccount" {
		logger.Info("TODO: Found, not updating, does not work, why? Secret accumulation?")
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

	if err := r.client.Update(context.TODO(), required); err != nil {
		return errs.Wrap(err, "Couldn't Update Resource")
	}

	return nil
}

func rebuildDriverContainer(obj *unstructured.Unstructured, r *ReconcileSpecialResource) error {

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
		logger.Info("No label driver-container-vendor found")
		return errs.New("No driver-container-vendor found, nor vendor == updateVendor")
	}

	return nil
}
