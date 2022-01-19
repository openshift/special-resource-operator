package poll

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"time"

	"github.com/go-logr/logr"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/lifecycle"
	"github.com/openshift-psap/special-resource-operator/pkg/storage"
	"github.com/openshift-psap/special-resource-operator/pkg/utils"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

//go:generate mockgen -source=poll.go -package=poll -destination=mock_poll_api.go

type PollActions interface {
	ForResourceUnavailability(context.Context, *unstructured.Unstructured) error
	ForResource(context.Context, *unstructured.Unstructured) error
	ForDaemonSet(context.Context, *unstructured.Unstructured) error
	ForDaemonSetLogs(context.Context, *unstructured.Unstructured, string) error
}

type pollActions struct {
	kubeClient clients.ClientsInterface
	lc         lifecycle.Lifecycle
	log        logr.Logger
	storage    storage.Storage
	waitFor    map[string]func(context.Context, *unstructured.Unstructured) error
}

const (
	retryInterval = time.Second * 5
	timeout       = time.Second * 30
)

func New(kubeClient clients.ClientsInterface, lc lifecycle.Lifecycle, storage storage.Storage) PollActions {
	actions := pollActions{
		kubeClient: kubeClient,
		lc:         lc,
		log:        zap.New(zap.UseDevMode(true)).WithName(utils.Print("wait", utils.Brown)),
		storage:    storage,
	}
	waitFor := map[string]func(context.Context, *unstructured.Unstructured) error{
		"Pod":                      actions.forPod,
		"DaemonSet":                actions.ForDaemonSet,
		"BuildConfig":              actions.forBuild,
		"Secret":                   actions.forSecret,
		"CustomResourceDefinition": actions.forCRD,
		"Job":                      actions.forJob,
		"Deployment":               actions.forDeployment,
		"StatefulSet":              actions.forStatefulSet,
		"Namespace":                actions.forResourceAvailability,
		"Certificates":             actions.forResourceAvailability,
	}
	actions.waitFor = waitFor
	return &actions
}

type statusCallback func(ctx context.Context, obj *unstructured.Unstructured) (bool, error)

func (p *pollActions) forResourceAvailability(ctx context.Context, obj *unstructured.Unstructured) error {

	found := obj.DeepCopy()
	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		err = p.kubeClient.Get(ctx, types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, found)
		if err != nil {
			if apierrors.IsNotFound(err) {
				p.log.Info("Waiting for creation of ", "Namespace", obj.GetNamespace(), "Name", obj.GetName())
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
	return err
}

func (p *pollActions) ForResourceUnavailability(ctx context.Context, obj *unstructured.Unstructured) error {

	found := obj.DeepCopy()
	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		err = p.kubeClient.Get(ctx, types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, found)
		if err != nil {
			if apierrors.IsNotFound(err) {
				p.log.Info("Waiting done for deletion of ", "Namespace", obj.GetNamespace(), "Name", obj.GetName())
				return true, nil
			}
			return true, err
		}
		p.log.Info("Waiting for deletion of ", "Namespace", obj.GetNamespace(), "Name", obj.GetName())
		return false, nil
	})
	return err
}

// makeStatusCallback Closure capturing json path and expected status
func makeStatusCallback(status interface{}, fields ...string) statusCallback {
	_status := status
	_fields := fields
	return func(_ context.Context, obj *unstructured.Unstructured) (bool, error) {
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

func (p *pollActions) ForResource(ctx context.Context, obj *unstructured.Unstructured) error {

	var err error
	// Wait for general availability, Pods Complete, Running
	// DaemonSet NumberUnavailable == 0, etc
	if wait, ok := p.waitFor[obj.GetKind()]; ok {
		p.log.Info("ForResource", "Kind", obj.GetKind())
		if err = wait(ctx, obj); err != nil {
			return errors.Wrap(err, "Waiting too long for resource")
		}
	} else {
		utils.WarnOnError(errors.New("No wait function registered for Kind: " + obj.GetKind()))
	}

	return nil
}

func (p *pollActions) forSecret(ctx context.Context, obj *unstructured.Unstructured) error {
	return p.forResourceAvailability(ctx, obj)
}

func (p *pollActions) forCRD(ctx context.Context, obj *unstructured.Unstructured) error {

	p.kubeClient.Invalidate()
	// Lets wait some time for the API server to register the new CRD
	if err := p.forResourceAvailability(ctx, obj); err != nil {
		return err
	}

	_, err := p.kubeClient.ServerGroups()
	utils.WarnOnError(err)

	return nil
}

func (p *pollActions) forPod(ctx context.Context, obj *unstructured.Unstructured) error {
	if err := p.forResourceAvailability(ctx, obj); err != nil {
		return err
	}
	callback := makeStatusCallback("Succeeded", "status", "phase")
	return p.forResourceFullAvailability(ctx, obj, callback)
}

func (p *pollActions) forDeployment(ctx context.Context, obj *unstructured.Unstructured) error {
	if err := p.forResourceAvailability(ctx, obj); err != nil {
		return err
	}
	return p.forResourceFullAvailability(ctx, obj, func(ctx context.Context, obj *unstructured.Unstructured) (bool, error) {

		labels, found, err := unstructured.NestedMap(obj.Object, "spec", "selector", "matchLabels")
		utils.WarnOnError(err)

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

		err = p.kubeClient.List(ctx, &rss, opts...)
		if err != nil {
			p.log.Error(err, "Could not get ReplicaSet", "Deployment", obj.GetName())
			return false, nil
		}

		for _, rs := range rss.Items {
			p.log.Info("Checking ReplicaSet", "name", rs.GetName())
			status, found, err := unstructured.NestedMap(rs.Object, "status")
			utils.WarnOnError(err)
			if !found {
				p.log.Info("No status for ReplicaSet", "name", rs.GetName())
				return false, nil
			}

			_, ok := status["replicas"]
			if !ok {
				p.log.Info("No replicas for ReplicaSet", "name", rs.GetName())
				return false, nil
			}
			repls := status["replicas"].(int64)
			_, okAvailableReplicas := status["availableReplicas"]
			if repls == 0 {
				p.log.Info("ReplicaSet scheduled for termination", "name", rs.GetName())
				continue
			}
			if !okAvailableReplicas {
				return false, nil
			}
			avail := status["availableReplicas"].(int64)
			p.log.Info("Status", "AvailableReplicas", avail, "Replicas", repls)
			if avail != repls {
				return false, nil
			}
		}
		return true, nil
	})
}

func (p *pollActions) forStatefulSet(ctx context.Context, obj *unstructured.Unstructured) error {
	if err := p.forResourceAvailability(ctx, obj); err != nil {
		return err
	}
	return p.forResourceFullAvailability(ctx, obj, func(_ context.Context, obj *unstructured.Unstructured) (bool, error) {

		repls, found, err := unstructured.NestedInt64(obj.Object, "spec", "replicas")
		utils.WarnOnError(err)
		if !found {
			return false, errors.New("Something went horribly wrong, cannot read .spec.replicas from StatefulSet")
		}
		p.log.Info("DEBUG", ".spec.replicas", repls)
		status, found, err := unstructured.NestedMap(obj.Object, "status")
		utils.WarnOnError(err)
		if !found {
			p.log.Info("No status for StatefulSet", "name", obj.GetName())
			return false, nil
		}
		if _, ok := status["currentReplicas"]; !ok {
			return false, nil
		}

		currt := status["currentReplicas"].(int64)

		if repls == currt {
			p.log.Info("Status", "Replicas", repls, "CurrentReplicas", currt)
			return true, nil
		}

		p.log.Info("Status", "Replicas", repls, "CurrentReplicas", currt)

		return false, nil
	})
}

func (p *pollActions) forJob(ctx context.Context, obj *unstructured.Unstructured) error {
	if err := p.forResourceAvailability(ctx, obj); err != nil {
		return err
	}

	return p.forResourceFullAvailability(ctx, obj, func(_ context.Context, obj *unstructured.Unstructured) (bool, error) {

		conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
		utils.WarnOnError(err)

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

func (p *pollActions) forDaemonSetCallback(ctx context.Context, obj *unstructured.Unstructured) (bool, error) {

	// The total number of nodes that should be running the daemon pod
	var callback statusCallback

	callback = func(_ context.Context, _ *unstructured.Unstructured) (bool, error) { return false, nil }

	desiredNumberScheduled, found, err := unstructured.NestedInt64(obj.Object, "status", "desiredNumberScheduled")
	if err != nil || !found {
		return found, err
	}

	_, found, _ = unstructured.NestedInt64(obj.Object, "status", "numberUnavailable")
	if found {
		callback = makeStatusCallback(0, "status", "numberUnavailable")
	}

	_, found, _ = unstructured.NestedInt64(obj.Object, "status", "numberAvailable")
	if found {
		callback = makeStatusCallback(desiredNumberScheduled, "status", "numberAvailable")
	}

	return callback(ctx, obj)
}

func (p *pollActions) forLifecycleAvailability(ctx context.Context, obj *unstructured.Unstructured) error {

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

	objKey := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}

	ins := types.NamespacedName{
		Namespace: os.Getenv("OPERATOR_NAMESPACE"),
		Name:      "special-resource-lifecycle",
	}

	return wait.Poll(retryInterval, timeout, func() (done bool, err error) {

		p.log.Info("Waiting for lifecycle update of ", "Namespace", obj.GetNamespace(), "Name", obj.GetName())

		pl := p.lc.GetPodFromDaemonSet(ctx, objKey)

		for _, pod := range pl.Items {
			p.log.Info("Checking lifecycle of", "Pod", pod.GetName())
			hs, err := utils.FNV64a(pod.GetNamespace() + pod.GetName())
			if err != nil {
				return false, err
			}
			value, err := p.storage.CheckConfigMapEntry(ctx, hs, ins)
			if err != nil {
				return false, err
			}
			if value != "" {
				return false, nil
			}
		}
		p.log.Info("All Pods running latest DaemonSet Template, we can move on")
		return true, nil
	})
}

func (p *pollActions) ForDaemonSet(ctx context.Context, obj *unstructured.Unstructured) error {
	if err := p.forResourceAvailability(ctx, obj); err != nil {
		return err
	}

	if err := p.forLifecycleAvailability(ctx, obj); err != nil {
		return err
	}

	return p.forResourceFullAvailability(ctx, obj, p.forDaemonSetCallback)
}

func (p *pollActions) forBuild(ctx context.Context, obj *unstructured.Unstructured) error {

	if err := p.forResourceAvailability(ctx, obj); err != nil {
		return err
	}

	builds := &unstructured.UnstructuredList{}
	builds.SetAPIVersion("build.openshift.io/v1")
	builds.SetKind("build")

	opts := []client.ListOption{
		client.InNamespace(clients.Namespace),
	}
	if err := p.kubeClient.List(ctx, builds, opts...); err != nil {
		return errors.Wrap(err, "Could not get BuildList")
	}

	for _, build := range builds.Items {
		callback := makeStatusCallback("Complete", "status", "phase")
		if err := p.forResourceFullAvailability(ctx, &build, callback); err != nil {
			return err
		}
	}

	return nil
}

func (p *pollActions) forResourceFullAvailability(ctx context.Context, obj *unstructured.Unstructured, callback statusCallback) error {

	found := obj.DeepCopy()

	return wait.Poll(retryInterval, timeout, func() (bool, error) {
		err := p.kubeClient.Get(ctx, types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, found)
		if err != nil {
			p.log.Error(err, "")
			return false, err
		}
		fnd, err := callback(ctx, found)
		if err != nil {
			return fnd, err
		}

		if fnd {
			p.log.Info("Resource available ", "Kind", obj.GetKind()+": "+obj.GetNamespace()+"/"+obj.GetName())
			return true, nil
		}
		p.log.Info("Waiting for availability of ", "Kind", obj.GetKind()+": "+obj.GetNamespace()+"/"+obj.GetName())
		return false, nil
	})
}

func (p *pollActions) ForDaemonSetLogs(ctx context.Context, obj *unstructured.Unstructured, pattern string) error {

	p.log.Info("WaitForDaemonSetLogs", "Name", obj.GetName())

	pods := &unstructured.UnstructuredList{}
	pods.SetAPIVersion("v1")
	pods.SetKind("pod")

	label := make(map[string]string)

	var found bool
	var selector string

	if selector, found = obj.GetLabels()["app"]; !found {
		return errors.New("Cannot find Label app=, missing take a look at the manifests")
	}

	p.log.Info("Looking for Pods with label app=" + selector)
	label["app"] = selector

	opts := []client.ListOption{
		client.InNamespace(clients.Namespace),
		client.MatchingLabels(label),
	}

	err := p.kubeClient.List(ctx, pods, opts...)
	if err != nil {
		return errors.Wrap(err, "Could not get PodList")
	}

	for _, pod := range pods.Items {
		p.log.Info("WaitForDaemonSetLogs", "Pod", pod.GetName())
		podLogOpts := v1.PodLogOptions{}
		req := p.kubeClient.GetPodLogs(pod.GetNamespace(), pod.GetName(), &podLogOpts)
		podLogs, err := req.Stream(ctx)
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
		p.log.Info("WaitForDaemonSetLogs", "LastBytes", lastBytes)

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
