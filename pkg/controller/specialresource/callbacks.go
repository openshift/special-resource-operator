package specialresource

import (
	"context"
	"fmt"

	errs "github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func init() {

	customCallback = make(resourceCallbacks)
	customCallback["specialresource-grafana-configmap"] = customGrafanaConfigMap
}

type resourceCallbacks map[string]func(obj *unstructured.Unstructured, r *ReconcileSpecialResource) error

var customCallback resourceCallbacks

func beforeCRUDhooks(obj *unstructured.Unstructured, r *ReconcileSpecialResource) error {

	var found bool
	todo := ""
	annotations := obj.GetAnnotations()

	if todo, found = annotations["specialresource.openshift.io/callback"]; !found {
		return nil
	}

	if prefix, ok := customCallback[todo]; ok {
		return prefix(obj, r)
	}
	return nil
}

func afterCRUDhooks(obj *unstructured.Unstructured, r *ReconcileSpecialResource) error {

	annotations := obj.GetAnnotations()

	if state, found := annotations["specialresource.openshift.io/state"]; found && state == "driver-container" {
		if err := checkForImagePullBackOff(obj, r); err != nil {
			return err
		}
	}

	if wait, found := annotations["specialresource.openshift.io/wait"]; found && wait == "true" {
		if err := waitForResource(obj, r); err != nil {
			return err
		}
	}

	if pattern, found := annotations["specialresrouce.openshift.io/wait-for-logs"]; found && len(pattern) > 0 {
		if err := waitForDaemonSetLogs(obj, r, pattern); err != nil {
			return err
		}
	}

	// If resource available, label the nodes according to the current state
	// if e.g driver-container ready -> specialresource.openshift.io/driver-container:ready
	return labelNodesAccordingToState(obj, r)
}

func checkForImagePullBackOff(obj *unstructured.Unstructured, r *ReconcileSpecialResource) error {

	if err := waitForDaemonSet(obj, r); err == nil {
		return nil
	}

	log.Info("checkForImagePullBackOff get pods")

	labels := obj.GetLabels()
	value := labels["app"]

	find := make(map[string]string)
	find["app"] = value

	// DaemonSet is not coming up, lets check if we have to rebuild
	pods := &unstructured.UnstructuredList{}
	pods.SetAPIVersion("v1")
	pods.SetKind("PodList")

	opts := &client.ListOptions{}
	opts.InNamespace(r.specialresource.Namespace)
	opts.MatchingLabels(find)
	log.Info("checkForImagePullBackOff get PodList")

	err := r.client.List(context.TODO(), opts, pods)
	if err != nil {
		log.Error(err, "Could not get PodList")
		return err
	}

	if len(pods.Items) == 0 {
		return fmt.Errorf("No Pods found, reconciling")
	}

	var reason string

	for _, pod := range pods.Items {
		log.Info("checkForImagePullBackOff", "PodName", pod.GetName())

		var err error
		var found bool
		var containerStatuses []interface{}

		if containerStatuses, found, err = unstructured.NestedSlice(pod.Object, "status", "containerStatuses"); !found || err != nil {
			phase, found, err := unstructured.NestedString(pod.Object, "status", "phase")
			checkNestedFields(found, err)
			log.Info("Pod is in phase: " + phase)
			continue
		}

		for _, containerStatus := range containerStatuses {
			switch containerStatus := containerStatus.(type) {
			case map[string]interface{}:
				reason, found, err = unstructured.NestedString(containerStatus, "state", "waiting", "reason")
				log.Info("Reason", "reason", reason)
			default:
				log.Info("checkForImagePullBackOff", "DEFAULT NOT THE CORRECT TYPE", containerStatus)
			}
			break
		}

		if reason == "ImagePullBackOff" || reason == "ErrImagePull" {
			annotations := obj.GetAnnotations()
			if vendor, ok := annotations["specialresource.openshift.io/driver-container-vendor"]; ok {
				runInfo.UpdateVendor = vendor
				return errs.New("ImagePullBackOff need to rebuild" + runInfo.UpdateVendor + "driver-container")
			}
		}

		log.Info("Unsetting updateVendor, Pods not in ImagePullBackOff or ErrImagePull")
		runInfo.UpdateVendor = ""
		return nil
	}

	return errs.New("Unexpected Phase of Pods in DameonSet: " + obj.GetName())
}
