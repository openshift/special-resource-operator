package controllers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	errs "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var waitFor resourceCallbacks

func init() {

	waitFor = make(resourceCallbacks)
	waitFor["Pod"] = waitForPod
	waitFor["DaemonSet"] = waitForDaemonSet
	waitFor["BuildConfig"] = waitForBuild
}

type statusCallback func(obj *unstructured.Unstructured) bool

var (
	retryInterval = time.Second * 5
	timeout       = time.Second * 15
)

// makeStatusCallback Closure capturing json path and expected status
func makeStatusCallback(obj *unstructured.Unstructured, status interface{}, fields ...string) func(obj *unstructured.Unstructured) bool {
	_status := status
	_fields := fields
	return func(obj *unstructured.Unstructured) bool {
		switch x := _status.(type) {
		case int64:
			expected := _status.(int64)
			current, found, err := unstructured.NestedInt64(obj.Object, _fields...)
			exitOnErrorOrNotFound(found, err)

			if current == int64(expected) {
				return true
			}
			return false

		case int:
			expected := _status.(int)
			current, found, err := unstructured.NestedInt64(obj.Object, _fields...)
			exitOnErrorOrNotFound(found, err)

			if int(current) == int(expected) {
				return true
			}
			return false

		case string:
			expected := _status.(string)
			current, found, err := unstructured.NestedString(obj.Object, _fields...)
			exitOnErrorOrNotFound(found, err)

			if stat := strings.Compare(current, expected); stat == 0 {
				return true
			}
			return false

		default:
			panic(fmt.Errorf("cannot extract type from %T", x))

		}
	}
}

var waitCallback resourceCallbacks

func waitForResource(obj *unstructured.Unstructured, r *SpecialResourceReconciler) error {

	log.Info("WaitForResource", "Kind", obj.GetKind())

	var err error = nil
	// Wait for general availability, Pods Complete, Running
	// DaemonSet NumberUnavailable == 0, etc
	if wait, ok := waitFor[obj.GetKind()]; ok {
		if err = wait(obj, r); err != nil {
			return errs.Wrap(err, "Waiting too long for resource")
		}
	}

	return nil
}

func waitForPod(obj *unstructured.Unstructured, r *SpecialResourceReconciler) error {
	if err := waitForResourceAvailability(obj, r); err != nil {
		return err
	}
	callback := makeStatusCallback(obj, "Succeeded", "status", "phase")
	return waitForResourceFullAvailability(obj, r, callback)
}

func waitForDaemonSetCallback(obj *unstructured.Unstructured) bool {

	// The total number of nodes that should be running the daemon pod
	var err error
	var found bool
	var callback statusCallback

	callback = func(obj *unstructured.Unstructured) bool { return false }

	node.count, found, err = unstructured.NestedInt64(obj.Object, "status", "desiredNumberScheduled")
	exitOnErrorOrNotFound(found, err)

	_, found, err = unstructured.NestedInt64(obj.Object, "status", "numberUnavailable")
	if found {
		callback = makeStatusCallback(obj, 0, "status", "numberUnavailable")
	}

	_, found, err = unstructured.NestedInt64(obj.Object, "status", "numberAvailable")
	if found {
		callback = makeStatusCallback(obj, node.count, "status", "numberAvailable")
	}

	return callback(obj)

}

func waitForDaemonSet(obj *unstructured.Unstructured, r *SpecialResourceReconciler) error {
	if err := waitForResourceAvailability(obj, r); err != nil {
		return err
	}

	return waitForResourceFullAvailability(obj, r, waitForDaemonSetCallback)
}

func waitForBuild(obj *unstructured.Unstructured, r *SpecialResourceReconciler) error {

	if err := waitForResourceAvailability(obj, r); err != nil {
		return err
	}

	builds := &unstructured.UnstructuredList{}
	builds.SetAPIVersion("build.openshift.io/v1")
	builds.SetKind("build")

	opts := []client.ListOption{
		client.InNamespace(r.specialresource.Spec.Metadata.Namespace),
	}
	if err := r.List(context.TODO(), builds, opts...); err != nil {
		return errs.Wrap(err, "Could not get BuildList")
	}

	for _, build := range builds.Items {
		callback := makeStatusCallback(&build, "Complete", "status", "phase")
		if err := waitForResourceFullAvailability(&build, r, callback); err != nil {
			return err
		}
	}

	return nil
}

func waitForResourceAvailability(obj *unstructured.Unstructured, r *SpecialResourceReconciler) error {

	found := obj.DeepCopy()
	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		err = r.Get(context.TODO(), types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, found)
		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Info("Waiting for creation of ", "Namespace", obj.GetNamespace(), "Name", obj.GetName())
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
	return err
}

func waitForResourceFullAvailability(obj *unstructured.Unstructured, r *SpecialResourceReconciler, callback statusCallback) error {

	found := obj.DeepCopy()

	if err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		err = r.Get(context.TODO(), types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, found)
		if err != nil {
			log.Error(err, "")
			return false, err
		}
		if callback(found) {
			log.Info("Resource available ", "Kind", obj.GetKind()+": "+obj.GetNamespace()+"/"+obj.GetName())
			return true, nil
		}
		log.Info("Waiting for availability of ", "Kind", obj.GetKind()+": "+obj.GetNamespace()+"/"+obj.GetName())
		return false, nil
	}); err != nil {
		return err
	}
	return nil
}

func waitForDaemonSetLogs(obj *unstructured.Unstructured, r *SpecialResourceReconciler, pattern string) error {

	log.Info("WaitForDaemonSetLogs", "Name", obj.GetName())

	pods := &unstructured.UnstructuredList{}
	pods.SetAPIVersion("v1")
	pods.SetKind("pod")

	label := make(map[string]string)

	var found bool
	var selector string

	if selector, found = obj.GetLabels()["app"]; !found {
		errs.New("Cannot find Label app=, missing take a look at the manifests")
	}

	log.Info("Looking for Pods with label app=" + selector)
	label["app"] = selector

	opts := []client.ListOption{
		client.InNamespace(r.specialresource.Spec.Metadata.Namespace),
		client.MatchingLabels(label),
	}

	err := r.List(context.TODO(), pods, opts...)
	if err != nil {
		return errs.Wrap(err, "Could not get PodList")
	}

	for _, pod := range pods.Items {
		log.Info("WaitForDaemonSetLogs", "Pod", pod.GetName())
		podLogOpts := corev1.PodLogOptions{}
		req := kubeclient.CoreV1().Pods(pod.GetNamespace()).GetLogs(pod.GetName(), &podLogOpts)
		podLogs, err := req.Stream(context.TODO())
		if err != nil {
			return errs.Wrap(err, "Error in opening stream")
		}
		defer podLogs.Close()

		buf := new(bytes.Buffer)
		_, err = io.Copy(buf, podLogs)
		if err != nil {
			return errs.Wrap(err, "Error in copy information from podLogs to buf")
		}

		cutoff := 100
		if buf.Len() <= 100 {
			cutoff = 0
		}

		logs := buf.String()
		lastBytes := logs[len(logs)-cutoff:]
		log.Info("WaitForDaemonSetLogs", "LastBytes", lastBytes)

		if match, _ := regexp.MatchString(pattern, lastBytes); !match {
			return errs.New("Not yet done. Not matched against: " + pattern)
		}
		// We're only checking one Pod not all of them
		break
	}

	return nil
}
