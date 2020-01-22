package specialresource

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	monitoringV1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/openshift-psap/special-resource-operator/pkg/yamlutil"
	buildV1 "github.com/openshift/api/build/v1"
	imageV1 "github.com/openshift/api/image/v1"
	routev1 "github.com/openshift/api/route/v1"
	secv1 "github.com/openshift/api/security/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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

// AddKubeClient Add a native non-caching client for advanced CRUD operations
func AddKubeClient(cfg *rest.Config) error {
	clientSet, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}
	kubeclient = clientSet
	return nil
}

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

func exitOnError(err error, msg string) {
	if err != nil {
		log.Error(err, msg)
		os.Exit(1)
	}
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
	opts.SetLabelSelector("feature.node.kubernetes.io/pci-10de.present=true")

	err := r.client.List(context.TODO(), opts, node.list)
	if err != nil {
		log.Error(err, "Could not get NodeList")
	}

	return node.list, err
}

func getSROstatesCM(r *ReconcileSpecialResource) (map[string]interface{}, []string, error) {

	log.Info("Looking for ConfigMap special-resource-operator-states")
	cm := &unstructured.Unstructured{}

	cm.SetAPIVersion("v1")
	cm.SetKind("ConfigMap")

	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: r.specialresource.GetNamespace(), Name: "special-resource-operator-states"}, cm)

	if apierrors.IsNotFound(err) {
		log.Info("ConfigMap special-resource-states not found, see README and create the states")
		return nil, nil, nil
	}

	manifests, found, err := unstructured.NestedMap(cm.Object, "data")
	checkNestedFields(found, err)

	states := make([]string, 0, len(manifests))
	for key := range manifests {
		states = append(states, key)
	}

	sort.Strings(states)

	return manifests, states, nil

}

// ReconcileClusterResources Reconcile cluster resources
func ReconcileClusterResources(r *ReconcileSpecialResource) error {

	manifests, states, err := getSROstatesCM(r)

	node.list, err = cacheNodes(r, false)
	exitOnError(err, "Cannot get Nodes")

	for _, state := range states {

		log.Info("Executing", "State", state)
		namespacedYAML := []byte(manifests[state].(string))
		if err := createFromYAML(namespacedYAML, r); err != nil {
			return err
		}
	}

	return nil
}

func createFromYAML(yamlFile []byte, r *ReconcileSpecialResource) error {

	namespace := r.specialresource.Namespace
	scanner := yamlutil.NewYAMLScanner(yamlFile)

	for scanner.Scan() {

		yamlSpec := scanner.Bytes()
		obj := &unstructured.Unstructured{}
		jsonSpec, err := yaml.YAMLToJSON(yamlSpec)
		if err != nil {
			return fmt.Errorf("could not convert yaml file to json: %v", err)
		}

		obj.UnmarshalJSON(jsonSpec)
		obj.SetNamespace(namespace)

		// Callbacks before CRUD will update the manifests
		if err := prefixResourceCallback(obj, r); err != nil {
			log.Error(err, "prefix callbacks exited non-zero")
			return err
		}
		// Create Update Delete Patch resources
		if err := CRUD(obj, r); err != nil {
			exitOnError(err, "CRUD exited non-zero")
		}
		// Callbacks after CRUD will wait for ressource and check status
		if err := postfixResourceCallback(obj, r); err != nil {
			log.Error(err, "postfix callbacks exited non-zero")
			return err
		}

	}

	if err := scanner.Err(); err != nil {
		log.Error(err, "failed to scan manifest: ", err)
		return err
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

func updateResource(req *unstructured.Unstructured, found *unstructured.Unstructured) error {

	kind := found.GetKind()

	if needToUpdateResourceVersion(kind) {
		version, fnd, err := unstructured.NestedString(found.Object, "metadata", "resourceVersion")
		checkNestedFields(fnd, err)

		if err := unstructured.SetNestedField(req.Object, version, "metadata", "resourceVersion"); err != nil {
			log.Error(err, "Couldn't update ResourceVersion")
			return err
		}

	}
	if kind == "Service" {
		clusterIP, fnd, err := unstructured.NestedString(found.Object, "spec", "clusterIP")
		checkNestedFields(fnd, err)

		if err := unstructured.SetNestedField(req.Object, clusterIP, "spec", "clusterIP"); err != nil {
			log.Error(err, "Couldn't update clusterIP")
			return err
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
		return fmt.Errorf("failed to set controller reference: (%v)", err)
	}

	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, found)

	if apierrors.IsNotFound(err) {
		logger.Info("Not found, creating")
		if err := r.client.Create(context.TODO(), obj); err != nil {
			logger.Error(err, "Couldn't Create Resource")
			return err
		}
		return nil
	}
	if err == nil && obj.GetKind() != "ServiceAccount" && obj.GetKind() != "Pod" {

		logger.Info("Found, updating")
		required := obj.DeepCopy()

		// required.ResourceVersion = found.ResourceVersion
		if err := updateResource(required, found); err != nil {
			logger.Error(err, "Couldn't Update ResourceVersion")
			return err
		}

		if err := r.client.Update(context.TODO(), required); err != nil {
			logger.Error(err, "Couldn't Update Resource")
			return err
		}
		return nil
	}

	if apierrors.IsForbidden(err) {
		logger.Error(err, "Forbidden check Role, ClusterRole and Bindings for operator")
		return err
	}

	if err != nil {
		logger.Error(err, "UNEXPECTED ERROR")
	}

	return nil
}
