package specialresource

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/openshift-psap/special-resource-operator/pkg/yamlutil"
	routev1 "github.com/openshift/api/route/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"
)

// Add3dpartyResourcesToScheme Adds 3rd party resources To the operator
func Add3dpartyResourcesToScheme(scheme *runtime.Scheme) error {

	if err := routev1.AddToScheme(scheme); err != nil {
		return err
	}

	return nil
}

// ReconcileClusterResources Reconcile cluster resources
func ReconcileClusterResources(r *ReconcileSpecialResource) error {

	// create namespaced resources
	namespacedYAML, err := ioutil.ReadFile("/etc/kubernetes/special-resource/nvidia-gpu/state-driver.yaml")
	if err != nil {
		return fmt.Errorf("failed to read namespaced manifest: %v", err)
	}
	return createFromYAML(namespacedYAML, r)
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

		if err := CRUD(obj, r); err != nil {
			log.Error(err, "CRUD exited non-zero")
			os.Exit(1)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan manifest: (%v)", err)
	}
	return nil
}

// CRUD Create Update Delete Resource
func CRUD(obj *unstructured.Unstructured, r *ReconcileSpecialResource) error {

	logger := log.WithValues("Kind", obj.GetKind(), "Namespace", obj.GetNamespace())
	found := obj.DeepCopy()

	if err := controllerutil.SetControllerReference(r.specialresource, obj, r.scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: (%v)", err)
	}

	logger.Info("Looking for")
	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, found)

	if err == nil {
		logger.Info("err == nil")
	}

	if err != nil {
		logger.Info("err != nil")
		logger.Error(err, "err != nil2")
	}

	if apierrors.IsNotFound(err) {
		logger.Info("Not found, creating")
		if err := r.client.Create(context.TODO(), obj); err != nil {
			return fmt.Errorf("Couldn't Create (%v)", err)
		}
	}
	if err == nil && obj.GetKind() != "ServiceAccount" {
		logger.Info("Found, updating")
		if err := r.client.Update(context.TODO(), obj); err != nil {
			return fmt.Errorf("Couldn't Update (%v)", err)
		}
	}
	return nil
}
