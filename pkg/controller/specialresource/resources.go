package specialresource

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"
)

var (
	manifests  = "/etc/kubernetes/special-resource/nvidia-gpu"
	kubeclient *kubernetes.Clientset
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

// ReconcileClusterResources Reconcile cluster resources
func ReconcileClusterResources(r *ReconcileSpecialResource) error {

	_, err := os.Stat(manifests)
	exitOnError(err, "Missing manifests dir: "+manifests)

	states, err := filePathWalkDir(manifests)
	exitOnError(err, "Cannot walk dir: "+manifests)

	for _, state := range states {

		log.Info("Executing", "State", state)
		namespacedYAML, err := ioutil.ReadFile(state)
		exitOnError(err, "Cannot read state file: "+state)

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
