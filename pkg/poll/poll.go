package poll

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/openshift-psap/special-resource-operator/pkg/cache"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"github.com/openshift-psap/special-resource-operator/pkg/exit"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	RetryInterval = time.Second * 5
	Timeout       = time.Second * 15
	log           logr.Logger
)

type pollCallbacks map[string]func(obj *unstructured.Unstructured) error

var (
	waitFor pollCallbacks
)

func init() {

	waitFor = make(pollCallbacks)
	waitFor["Pod"] = ForPod
	waitFor["DaemonSet"] = ForDaemonSet
	waitFor["BuildConfig"] = ForBuild
	waitFor["Secret"] = ForSecret
	log = zap.New(zap.UseDevMode(true)).WithName(color.Print("wait", color.Brown))
}

type statusCallback func(obj *unstructured.Unstructured) bool

func init() {

}

func ForResourceAvailability(obj *unstructured.Unstructured) error {

	found := obj.DeepCopy()
	err := wait.Poll(RetryInterval, Timeout, func() (done bool, err error) {
		err = clients.Interface.Get(context.TODO(), types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, found)
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

// makeStatusCallback Closure capturing json path and expected status
func makeStatusCallback(obj *unstructured.Unstructured, status interface{}, fields ...string) func(obj *unstructured.Unstructured) bool {
	_status := status
	_fields := fields
	return func(obj *unstructured.Unstructured) bool {
		switch x := _status.(type) {
		case int64:
			expected := _status.(int64)
			current, found, err := unstructured.NestedInt64(obj.Object, _fields...)
			exit.OnErrorOrNotFound(found, err)

			if current == int64(expected) {
				return true
			}
			return false

		case int:
			expected := _status.(int)
			current, found, err := unstructured.NestedInt64(obj.Object, _fields...)
			exit.OnErrorOrNotFound(found, err)

			if int(current) == int(expected) {
				return true
			}
			return false

		case string:
			expected := _status.(string)
			current, found, err := unstructured.NestedString(obj.Object, _fields...)
			exit.OnErrorOrNotFound(found, err)

			if stat := strings.Compare(current, expected); stat == 0 {
				return true
			}
			return false

		default:
			panic(fmt.Errorf("cannot extract type from %T", x))

		}
	}
}

func ForResource(obj *unstructured.Unstructured) error {

	log.Info("ForResource", "Kind", obj.GetKind())

	var err error
	// Wait for general availability, Pods Complete, Running
	// DaemonSet NumberUnavailable == 0, etc
	if wait, ok := waitFor[obj.GetKind()]; ok {
		if err = wait(obj); err != nil {
			return errors.Wrap(err, "Waiting too long for resource")
		}
	}

	return nil
}

func ForSecret(obj *unstructured.Unstructured) error {
	return ForResourceAvailability(obj)
}

func ForPod(obj *unstructured.Unstructured) error {
	if err := ForResourceAvailability(obj); err != nil {
		return err
	}
	callback := makeStatusCallback(obj, "Succeeded", "status", "phase")
	return ForResourceFullAvailability(obj, callback)
}

func ForDaemonSetCallback(obj *unstructured.Unstructured) bool {

	// The total number of nodes that should be running the daemon pod
	var err error
	var found bool
	var callback statusCallback

	callback = func(obj *unstructured.Unstructured) bool { return false }

	cache.Node.Count, found, err = unstructured.NestedInt64(obj.Object, "status", "desiredNumberScheduled")
	exit.OnErrorOrNotFound(found, err)

	_, found, _ = unstructured.NestedInt64(obj.Object, "status", "numberUnavailable")
	if found {
		callback = makeStatusCallback(obj, 0, "status", "numberUnavailable")
	}

	_, found, _ = unstructured.NestedInt64(obj.Object, "status", "numberAvailable")
	if found {
		callback = makeStatusCallback(obj, cache.Node.Count, "status", "numberAvailable")
	}

	return callback(obj)

}

func ForDaemonSet(obj *unstructured.Unstructured) error {
	if err := ForResourceAvailability(obj); err != nil {
		return err
	}

	return ForResourceFullAvailability(obj, ForDaemonSetCallback)
}

func ForBuild(obj *unstructured.Unstructured) error {

	if err := ForResourceAvailability(obj); err != nil {
		return err
	}

	builds := &unstructured.UnstructuredList{}
	builds.SetAPIVersion("build.openshift.io/v1")
	builds.SetKind("build")

	opts := []client.ListOption{
		client.InNamespace(clients.Namespace),
	}
	if err := clients.Interface.List(context.TODO(), builds, opts...); err != nil {
		return errors.Wrap(err, "Could not get BuildList")
	}

	for _, build := range builds.Items {
		callback := makeStatusCallback(&build, "Complete", "status", "phase")
		if err := ForResourceFullAvailability(&build, callback); err != nil {
			return err
		}
	}

	return nil
}

func ForResourceFullAvailability(obj *unstructured.Unstructured, callback statusCallback) error {

	found := obj.DeepCopy()

	if err := wait.Poll(RetryInterval, Timeout, func() (done bool, err error) {
		err = clients.Interface.Get(context.TODO(), types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, found)
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

func ForDaemonSetLogs(obj *unstructured.Unstructured, pattern string) error {

	log.Info("WaitForDaemonSetLogs", "Name", obj.GetName())

	pods := &unstructured.UnstructuredList{}
	pods.SetAPIVersion("v1")
	pods.SetKind("pod")

	label := make(map[string]string)

	var found bool
	var selector string

	if selector, found = obj.GetLabels()["app"]; !found {
		return errors.New("Cannot find Label app=, missing take a look at the manifests")
	}

	log.Info("Looking for Pods with label app=" + selector)
	label["app"] = selector

	opts := []client.ListOption{
		client.InNamespace(clients.Namespace),
		client.MatchingLabels(label),
	}

	err := clients.Interface.List(context.TODO(), pods, opts...)
	if err != nil {
		return errors.Wrap(err, "Could not get PodList")
	}

	for _, pod := range pods.Items {
		log.Info("WaitForDaemonSetLogs", "Pod", pod.GetName())
		podLogOpts := v1.PodLogOptions{}
		req := clients.Interface.CoreV1().Pods(pod.GetNamespace()).GetLogs(pod.GetName(), &podLogOpts)
		podLogs, err := req.Stream(context.TODO())
		if err != nil {
			return errors.Wrap(err, "Error in opening stream")
		}
		defer podLogs.Close()

		buf := new(bytes.Buffer)
		_, err = io.Copy(buf, podLogs)
		if err != nil {
			return errors.Wrap(err, "Error in copy information from podLogs to buf")
		}

		cutoff := 100
		if buf.Len() <= 100 {
			cutoff = 0
		}

		logs := buf.String()
		lastBytes := logs[len(logs)-cutoff:]
		log.Info("WaitForDaemonSetLogs", "LastBytes", lastBytes)

		if match, _ := regexp.MatchString(pattern, lastBytes); !match {
			return errors.New("Not yet done. Not matched against: " + pattern)
		}
	}

	return nil
}
