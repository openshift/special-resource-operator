package specialresource

import (
	"os"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type resourceCallbacks map[string]func(obj *unstructured.Unstructured, r *ReconcileSpecialResource) error

var prefixCallback resourceCallbacks
var postfixCallback resourceCallbacks

// SetupCallbacks preassign callbacks for manipulating and handling of resources
func SetupCallbacks() error {

	prefixCallback = make(resourceCallbacks)
	postfixCallback = make(resourceCallbacks)

	prefixCallback["nvidia-driver-daemonset"] = prefixNVIDIAdriverDaemonset
	prefixCallback["nvidia-grafana-configmap"] = prefixNVIDIAgrafanaConfigMap

	return nil
}

func checkNestedFields(found bool, err error) {
	if !found || err != nil {
		log.Error(err, "")
		os.Exit(1)
	}
}

func prefixResourceCallback(obj *unstructured.Unstructured, r *ReconcileSpecialResource) error {

	var ok bool
	todo := ""
	annotations := obj.GetAnnotations()

	if todo, ok = annotations["callback"]; !ok {
		return nil
	}

	if prefix, ok := prefixCallback[todo]; ok {
		return prefix(obj, r)
	}
	return nil
}

func postfixResourceCallback(obj *unstructured.Unstructured, r *ReconcileSpecialResource) error {

	var ok bool
	todo := ""
	annotations := obj.GetAnnotations()
	todo = annotations["callback"]

	if todo, ok = annotations["callback"]; !ok {
		return nil
	}
	if postfix, ok := postfixCallback[todo]; ok {
		return postfix(obj, r)
	}

	if err := waitForResource(obj, r); err != nil {
		return err
	}

	return nil
}
