package poll

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"time"

	"github.com/go-logr/logr"
	"github.com/openshift-psap/special-resource-operator/pkg/cache"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"github.com/openshift-psap/special-resource-operator/pkg/lifecycle"
	"github.com/openshift-psap/special-resource-operator/pkg/warn"

	"github.com/pkg/errors"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	RetryInterval = time.Second * 5
	Timeout       = time.Second * 30
	log           logr.Logger
	waitFor       = map[string]func(obj *unstructured.Unstructured) error{
		"Pod":                      ForPod,
		"DaemonSet":                ForDaemonSet,
		"BuildConfig":              ForBuild,
		"Secret":                   ForSecret,
		"CustomResourceDefinition": ForCRD,
		"Job":                      ForJob,
		"Deployment":               ForDeployment,
		"StatefulSet":              ForStatefulSet,
		"Namespace":                ForResourceAvailability,
		"Certificates":             ForResourceAvailability,
	}
)

func init() {
	log = zap.New(zap.UseDevMode(true)).WithName(color.Print("wait", color.Brown))
}

type statusCallback func(obj *unstructured.Unstructured) (bool, error)

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

func ForResourceUnavailability(obj *unstructured.Unstructured) error {

	found := obj.DeepCopy()
	err := wait.Poll(RetryInterval, Timeout, func() (done bool, err error) {
		err = clients.Interface.Get(context.TODO(), types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, found)
		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Info("Waiting done for deletion of ", "Namespace", obj.GetNamespace(), "Name", obj.GetName())
				return true, nil
			}
			return true, err
		}
		log.Info("Waiting for deletion of ", "Namespace", obj.GetNamespace(), "Name", obj.GetName())
		return false, nil
	})
	return err
}

// makeStatusCallback Closure capturing json path and expected status
func makeStatusCallback(
	obj *unstructured.Unstructured,
	status interface{}, fields ...string) statusCallback {
	_status := status
	_fields := fields
	return func(obj *unstructured.Unstructured) (bool, error) {
		switch expected := _status.(type) {
		case int64:
			current, found, err := unstructured.NestedInt64(obj.Object, _fields...)
			if err != nil || !found {
				return false, fmt.Errorf("error or not found: %w", err)
			}

			return current == expected, nil

		case int:
			current, found, err := unstructured.NestedInt64(obj.Object, _fields...)
			if err != nil || !found {
				return false, fmt.Errorf("error or not found: %w", err)
			}

			return int(current) == expected, nil

		case string:
			current, found, err := unstructured.NestedString(obj.Object, _fields...)
			if err != nil || !found {
				return false, fmt.Errorf("error or not found: %w", err)
			}

			return current == expected, nil

		default:
			return false, fmt.Errorf("%T: unhandled type", _status)
		}
	}
}

func ForResource(obj *unstructured.Unstructured) error {

	var err error
	// Wait for general availability, Pods Complete, Running
	// DaemonSet NumberUnavailable == 0, etc
	if wait, ok := waitFor[obj.GetKind()]; ok {
		log.Info("ForResource", "Kind", obj.GetKind())
		if err = wait(obj); err != nil {
			return errors.Wrap(err, "Waiting too long for resource")
		}
	} else {
		warn.OnError(errors.New("No wait function registered for Kind: " + obj.GetKind()))
	}

	return nil
}

func ForSecret(obj *unstructured.Unstructured) error {
	return ForResourceAvailability(obj)
}

func ForCRD(obj *unstructured.Unstructured) error {

	clients.Interface.Invalidate()
	// Lets wait some time for the API server to register the new CRD
	if err := ForResourceAvailability(obj); err != nil {
		return err
	}

	_, err := clients.Interface.ServerGroups()
	warn.OnError(err)

	return nil
}

func ForPod(obj *unstructured.Unstructured) error {
	if err := ForResourceAvailability(obj); err != nil {
		return err
	}
	callback := makeStatusCallback(obj, "Succeeded", "status", "phase")
	return ForResourceFullAvailability(obj, callback)
}

func ForDeployment(obj *unstructured.Unstructured) error {
	if err := ForResourceAvailability(obj); err != nil {
		return err
	}
	return ForResourceFullAvailability(obj, func(obj *unstructured.Unstructured) (bool, error) {

		labels, found, err := unstructured.NestedMap(obj.Object, "spec", "selector", "matchLabels")
		warn.OnError(err)

		if !found {
			return false, err
		}

		matchingLabels := make(map[string]string)
		for k, v := range labels {
			matchingLabels[k] = v.(string)
		}

		opts := []client.ListOption{
			client.InNamespace(obj.GetNamespace()),
			client.MatchingLabels(matchingLabels),
		}
		rss := unstructured.UnstructuredList{}
		rss.SetKind("ReplicaSetList")
		rss.SetAPIVersion("apps/v1")

		err = clients.Interface.List(context.TODO(), &rss, opts...)
		if err != nil {
			log.Info("Could not get ReplicaSet", "Deployment", obj.GetName(), "error", err)
			return false, nil
		}

		for _, rs := range rss.Items {
			log.Info("Checking ReplicaSet", "name", rs.GetName())
			status, found, err := unstructured.NestedMap(rs.Object, "status")
			warn.OnError(err)
			if !found {
				log.Info("No status for ReplicaSet", "name", rs.GetName())
				return false, nil
			}

			_, ok := status["replicas"]
			if !ok {
				log.Info("No replicas for ReplicaSet", "name", rs.GetName())
				return false, nil
			}
			repls := status["replicas"].(int64)
			_, okAvailableReplicas := status["availableReplicas"]
			if repls == 0 {
				log.Info("ReplicaSet scheduled for termination", "name", rs.GetName())
				continue
			}
			if !okAvailableReplicas {
				return false, nil
			}
			avail := status["availableReplicas"].(int64)
			log.Info("Status", "AvailableReplicas", avail, "Replicas", repls)
			if avail != repls {
				return false, nil
			}
		}
		return true, nil
	})
}

func ForStatefulSet(obj *unstructured.Unstructured) error {
	if err := ForResourceAvailability(obj); err != nil {
		return err
	}
	return ForResourceFullAvailability(obj, func(obj *unstructured.Unstructured) (bool, error) {

		repls, found, err := unstructured.NestedInt64(obj.Object, "spec", "replicas")
		warn.OnError(err)
		if !found {
			return false, errors.New("Something went horribly wrong, cannot read .spec.replicas from StatefulSet")
		}
		log.Info("DEBUG", ".spec.replicas", repls)
		status, found, err := unstructured.NestedMap(obj.Object, "status")
		warn.OnError(err)
		if !found {
			log.Info("No status for StatefulSet", "name", obj.GetName())
			return false, nil
		}
		if _, ok := status["currentReplicas"]; !ok {
			return false, nil
		}

		currt := status["currentReplicas"].(int64)

		if repls == currt {
			log.Info("Status", "Replicas", repls, "CurrentReplicas", currt)
			return true, nil
		}

		log.Info("Status", "Replicas", repls, "CurrentReplicas", currt)

		return false, nil
	})
}

func ForJob(obj *unstructured.Unstructured) error {
	if err := ForResourceAvailability(obj); err != nil {
		return err
	}

	return ForResourceFullAvailability(obj, func(obj *unstructured.Unstructured) (bool, error) {

		conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
		warn.OnError(err)

		if !found {
			return false, nil
		}

		for _, condition := range conditions {

			status, found, err := unstructured.NestedString(condition.(map[string]interface{}), "status")
			if err != nil || !found {
				return false, fmt.Errorf("error or not found: %w", err)
			}

			if status == "True" {
				stype, found, err := unstructured.NestedString(condition.(map[string]interface{}), "type")
				if err != nil || !found {
					return false, fmt.Errorf("error or not found: %w", err)
				}

				if stype == "Complete" {
					return true, nil
				}
			}

		}
		return false, nil
	})
}

func ForDaemonSetCallback(obj *unstructured.Unstructured) (bool, error) {

	// The total number of nodes that should be running the daemon pod
	var err error
	var found bool
	var callback statusCallback

	callback = func(obj *unstructured.Unstructured) (bool, error) { return false, nil }

	cache.Node.Count, found, err = unstructured.NestedInt64(obj.Object, "status", "desiredNumberScheduled")
	if err != nil || !found {
		return found, err
	}

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

func ForLifecycleAvailability(obj *unstructured.Unstructured) error {

	if obj.GetKind() != "DaemonSet" {
		return nil
	}

	strategy, found, err := unstructured.NestedString(obj.Object, "spec", "updateStrategy", "type")
	if err != nil {
		return err
	}

	if !found || strategy != "OnDelete" {
		return nil
	}

	annotations := obj.GetAnnotations()
	tempGenerator := annotations[apps.DeprecatedTemplateGeneration]

	objKey := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}

	return wait.Poll(RetryInterval, Timeout, func() (done bool, err error) {

		log.Info("Waiting for lifecycle update of ", "Namespace", obj.GetNamespace(), "Name", obj.GetName())

		pl := lifecycle.GetPodFromDaemonSet(objKey)

		for _, pod := range pl.Items {
			log.Info("Checking lifecycle of", "Pod", pod.GetName())
			podLabels := pod.GetLabels()
			podGenerator := podLabels[extensions.DaemonSetTemplateGenerationKey]
			if podGenerator != tempGenerator {
				return false, nil
			}
		}
		log.Info("All Pods running latest DaemonSet Template, we can move on")
		return true, nil
	})
}

func ForDaemonSet(obj *unstructured.Unstructured) error {
	if err := ForResourceAvailability(obj); err != nil {
		return err
	}

	if err := ForLifecycleAvailability(obj); err != nil {
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

	return wait.Poll(RetryInterval, Timeout, func() (bool, error) {
		err := clients.Interface.Get(context.TODO(), types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, found)
		if err != nil {
			log.Error(err, "")
			return false, err
		}
		fnd, err := callback(found)
		if err != nil {
			return fnd, err
		}

		if fnd {
			log.Info("Resource available ", "Kind", obj.GetKind()+": "+obj.GetNamespace()+"/"+obj.GetName())
			return true, nil
		}
		log.Info("Waiting for availability of ", "Kind", obj.GetKind()+": "+obj.GetNamespace()+"/"+obj.GetName())
		return false, nil
	})
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
			return fmt.Errorf("error in opening stream: %w", err)
		}
		defer podLogs.Close()

		buf := new(bytes.Buffer)

		if _, err = io.Copy(buf, podLogs); err != nil {
			return fmt.Errorf("error in copy information from podLogs to buf: %w", err)
		}

		cutoff := 100
		if buf.Len() <= 100 {
			cutoff = 0
		}

		logs := buf.String()
		lastBytes := logs[len(logs)-cutoff:]
		log.Info("WaitForDaemonSetLogs", "LastBytes", lastBytes)

		var match bool

		match, err = regexp.MatchString(pattern, lastBytes)
		if err != nil {
			return fmt.Errorf("error matching pattern %q: %v", pattern, err)
		}

		if !match {
			return fmt.Errorf("not yet done; not matched against %q", pattern)
		}
	}

	return nil
}
