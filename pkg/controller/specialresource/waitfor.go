package specialresource

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type statusCallback func(obj *unstructured.Unstructured) bool

var stateLabels = map[string]map[string]string{
	"nvidia-driver-daemonset":            {"specialresource.openshift.io/driver-container": "ready"},
	"nvidia-driver-validation-daemonset": {"specialresource.openshift.io/driver-validation": "ready"},
	"nvidia-device-plugin-daemonset":     {"specialresource.openshift.io/device-plugin": "ready"},
	"nvidia-dp-validation-daemonset":     {"specialresource.openshift.io/device-validation": "ready"},
	"nvidia-dcgm-exporter":               {"specialresource.openshift.io/device-monitoring": "ready"},
}

// makeStatusCallback Closure capturing json path and expected status
func makeStatusCallback(obj *unstructured.Unstructured, status interface{}, fields ...string) func(obj *unstructured.Unstructured) bool {
	_status := status
	_fields := fields
	return func(obj *unstructured.Unstructured) bool {
		switch x := _status.(type) {
		case int64:
			expected := _status.(int64)
			current, found, err := unstructured.NestedInt64(obj.Object, _fields...)
			checkNestedFields(found, err)

			if current == int64(expected) {
				return true
			}
			return false

		case int:
			expected := _status.(int)
			current, found, err := unstructured.NestedInt64(obj.Object, _fields...)
			checkNestedFields(found, err)

			if int(current) == int(expected) {
				return true
			}
			return false

		case string:
			expected := _status.(string)
			current, found, err := unstructured.NestedString(obj.Object, _fields...)
			checkNestedFields(found, err)

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

func labelNodesAccordingToState(obj *unstructured.Unstructured, r *ReconcileSpecialResource) error {

	if obj.GetKind() != "DaemonSet" {
		return nil
	}

	cacheNodes(r, true)

	for _, node := range node.list.Items {
		labels := node.GetLabels()

		stateLabel, found := stateLabels[obj.GetName()]
		if !found {
			return nil
		}

		for k := range stateLabel {

			_, found := labels[k]
			if found {
				log.Info("NODE", "Label ", stateLabel, "on ", node.GetName())
				updateStatus(obj, r, stateLabel)
				continue
			}
			// Label missing update the Node to advance to the next state
			updated := node.DeepCopy()

			labels[k] = "ready"

			updated.SetLabels(labels)

			err := r.client.Update(context.TODO(), updated)
			if apierrors.IsForbidden(err) {
				return fmt.Errorf("Forbidden check Role, ClusterRole and Bindings for operator %s", err)
			}
			if apierrors.IsConflict(err) {
				cacheNodes(r, true)
				return fmt.Errorf("Node Conflict Label %s err %s", stateLabel, err)
			}

			if err != nil {
				log.Error(err, "Node Update", "label", stateLabel)
				return fmt.Errorf("Couldn't Update Node")
			}

			log.Info("NODE", "Setting Label ", stateLabel, "on ", updated.GetName())

			updateStatus(obj, r, stateLabel)
		}
	}
	return nil
}

func waitForResource(obj *unstructured.Unstructured, r *ReconcileSpecialResource) error {

	log.Info("waitForResource", "Kind", obj.GetKind())

	var err error = nil
	// Wait for general availability, Pods Complete, Running
	// DaemonSet NumberUnavailable == 0, etc
	if wait, ok := waitFor[obj.GetKind()]; ok {
		if err = wait(obj, r); err != nil {
			return err
		}
	}
	// Wait for specific condition of a specific resource
	if wait, ok := waitFor[obj.GetName()]; ok {
		if err = wait(obj, r); err != nil {
			return err
		}
	}
	// If resource available, label the nodes according to the current state
	// if e.g driver-container ready -> specialresource.openshift.io/driver-container:ready
	return labelNodesAccordingToState(obj, r)
}

func waitForPod(obj *unstructured.Unstructured, r *ReconcileSpecialResource) error {
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
	checkNestedFields(found, err)

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

func waitForDaemonSet(obj *unstructured.Unstructured, r *ReconcileSpecialResource) error {
	if err := waitForResourceAvailability(obj, r); err != nil {
		return err
	}

	return waitForResourceFullAvailability(obj, r, waitForDaemonSetCallback)
}

func waitForBuild(obj *unstructured.Unstructured, r *ReconcileSpecialResource) error {

	if err := waitForResourceAvailability(obj, r); err != nil {
		return err
	}

	builds := &unstructured.UnstructuredList{}
	builds.SetAPIVersion("build.openshift.io/v1")
	builds.SetKind("build")

	opts := &client.ListOptions{}
	opts.InNamespace(r.specialresource.Namespace)

	err := r.client.List(context.TODO(), opts, builds)
	if err != nil {
		log.Error(err, "Could not get BuildList")
		return err
	}

	for _, build := range builds.Items {
		callback := makeStatusCallback(&build, "Complete", "status", "phase")
		err := waitForResourceFullAvailability(&build, r, callback)
		if err != nil {
			return err
		}
	}

	return nil
}

// WAIT FOR RESOURCES -- other file?

var (
	retryInterval = time.Second * 5
	timeout       = time.Second * 120
)

func waitForResourceAvailability(obj *unstructured.Unstructured, r *ReconcileSpecialResource) error {

	found := obj.DeepCopy()
	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		err = r.client.Get(context.TODO(), types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, found)
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

func waitForResourceFullAvailability(obj *unstructured.Unstructured, r *ReconcileSpecialResource, callback statusCallback) error {

	found := obj.DeepCopy()

	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		err = r.client.Get(context.TODO(), types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, found)
		if err != nil {
			log.Error(err, "")
			return false, err
		}
		if callback(found) {
			log.Info("Resource available ", "Namespace", obj.GetNamespace(), "Name", obj.GetName())
			return true, nil
		}
		log.Info("Waiting for availability of ", "Namespace", obj.GetNamespace(), "Name", obj.GetName())
		return false, nil
	})
	return err
}

func waitForDaemonSetLogs(obj *unstructured.Unstructured, r *ReconcileSpecialResource) error {
	log.Info("waitForDaemonSetLogs", "Name", obj.GetName())

	pods := &unstructured.UnstructuredList{}
	pods.SetAPIVersion("v1")
	pods.SetKind("pod")

	label := make(map[string]string)
	label["app"] = obj.GetName()

	opts := &client.ListOptions{}
	opts.InNamespace(r.specialresource.Namespace)
	opts.MatchingLabels(label)

	err := r.client.List(context.TODO(), opts, pods)
	if err != nil {
		log.Error(err, "Could not get PodList")
		return err
	}

	for _, pod := range pods.Items {
		log.Info("waitForDaemonSetLogs", "Pod", pod.GetName())
		podLogOpts := corev1.PodLogOptions{}
		req := kubeclient.CoreV1().Pods(pod.GetNamespace()).GetLogs(pod.GetName(), &podLogOpts)
		podLogs, err := req.Stream()
		if err != nil {
			log.Error(err, "Error in opening stream")
			return err
		}
		defer podLogs.Close()

		buf := new(bytes.Buffer)
		_, err = io.Copy(buf, podLogs)
		if err != nil {
			log.Error(err, "Error in copy information from podLogs to buf")
			return err
		}
		str := buf.String()
		lastBytes := str[len(str)-100:]
		log.Info("waitForDaemonSetLogs", "LastBytes", lastBytes)
		pattern := "\\+ wait \\d+"
		if match, _ := regexp.MatchString(pattern, lastBytes); !match {
			return errors.New("Not yet done. Not matched against \\+ wait \\d+ ")
		}

		// We're only checking one Pod not all of them
		break
	}

	return nil
}
