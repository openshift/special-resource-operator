package controllers

import (
	"context"
	"fmt"

	errs "github.com/pkg/errors"
	client "sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func init() {

	customCallback = make(resourceCallbacks)
	customCallback["specialresource-grafana-configmap"] = customGrafanaConfigMap
}

type resourceCallbacks map[string]func(obj *unstructured.Unstructured, r *SpecialResourceReconciler) error

var customCallback resourceCallbacks

func beforeCRUDhooks(obj *unstructured.Unstructured, r *SpecialResourceReconciler) error {

	var found bool
	todo := ""
	annotations := obj.GetAnnotations()

	if proxy, found := annotations["specialresource.openshift.io/proxy"]; found && proxy == "true" {
		if err := setupProxy(obj, r); err != nil {
			return errs.Wrap(err, "Could not setup Proxy")
		}
	}

	if todo, found = annotations["specialresource.openshift.io/callback"]; !found {
		return nil
	}

	if prefix, ok := customCallback[todo]; ok {
		if err := prefix(obj, r); err != nil {
			return errs.Wrap(err, "Could not run prefix callback")
		}
	}
	return nil
}

func afterCRUDhooks(obj *unstructured.Unstructured, r *SpecialResourceReconciler) error {

	annotations := obj.GetAnnotations()
	for key, element := range annotations {
		log.Info("Annotations", "Key:", key, "Element:", element)
	}

	if state, found := annotations["specialresource.openshift.io/state"]; found && state == "driver-container" {
		log.Info("specialresource.openshift.io/state")
		if err := checkForImagePullBackOff(obj, r); err != nil {
			return errs.Wrap(err, "Cannot check for ImagePullBackOff")
		}
	}

	if wait, found := annotations["specialresource.openshift.io/wait"]; found && wait == "true" {
		log.Info("specialresource.openshift.io/wait")
		if err := waitForResource(obj, r); err != nil {
			return errs.Wrap(err, "Could not wait for resource")
		}
	}

	if pattern, found := annotations["specialresource.openshift.io/wait-for-logs"]; found && len(pattern) > 0 {
		log.Info("specialresource.openshift.io/wait-for-logs")
		if err := waitForDaemonSetLogs(obj, r, pattern); err != nil {
			return errs.Wrap(err, "Could not wait for DaemonSet logs")
		}
	}

	// If resource available, label the nodes according to the current state
	// if e.g driver-container ready -> specialresource.openshift.io/driver-container:ready
	return labelNodesAccordingToState(obj, r)
}

func checkForImagePullBackOff(obj *unstructured.Unstructured, r *SpecialResourceReconciler) error {

	if err := waitForDaemonSet(obj, r); err == nil {
		return nil
	}

	labels := obj.GetLabels()
	value := labels["app"]

	find := make(map[string]string)
	find["app"] = value

	// DaemonSet is not coming up, lets check if we have to rebuild
	pods := &unstructured.UnstructuredList{}
	pods.SetAPIVersion("v1")
	pods.SetKind("PodList")

	log.Info("checkForImagePullBackOff get PodList from: " + r.specialresource.Spec.Namespace)

	opts := []client.ListOption{
		client.InNamespace(r.specialresource.Spec.Namespace),
		client.MatchingLabels(find),
	}

	err := r.List(context.TODO(), pods, opts...)
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
			exitOnErrorOrNotFound(found, err)
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
