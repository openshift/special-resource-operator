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

// makeStatusCallback Closure capturing json path and expected status
func makeStatusCallback(obj *unstructured.Unstructured, status interface{}, fields ...string) func(obj *unstructured.Unstructured) bool {
	_status := status
	_fields := fields
	return func(obj *unstructured.Unstructured) bool {
		switch x := _status.(type) {
		case int, int32, int8, int64:

			expected := _status.(int)
			current, found, _ := unstructured.NestedInt64(obj.Object, _fields...)
			if !found {
				log.Info("Not found, ignoring")
				return true
			}
			if current == int64(expected) {
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
	return err
}

func waitForPod(obj *unstructured.Unstructured, r *ReconcileSpecialResource) error {
	if err := waitForResourceAvailability(obj, r); err != nil {
		return err
	}
	callback := makeStatusCallback(obj, "Succeeded", "status", "phase")
	return waitForResourceFullAvailability(obj, r, callback)
}

func waitForDaemonSet(obj *unstructured.Unstructured, r *ReconcileSpecialResource) error {
	if err := waitForResourceAvailability(obj, r); err != nil {
		return err
	}
	callback := makeStatusCallback(obj, 0, "status", "numberUnavailable")
	return waitForResourceFullAvailability(obj, r, callback)
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
		lastBytes := str[len(str)-20:]
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
