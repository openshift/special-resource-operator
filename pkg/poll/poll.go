package poll

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/openshift/special-resource-operator/pkg/clients"
	"github.com/openshift/special-resource-operator/pkg/lifecycle"
	"github.com/openshift/special-resource-operator/pkg/storage"
	"github.com/openshift/special-resource-operator/pkg/utils"

	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	storage    storage.Storage
	waitFor    map[string]func(context.Context, *unstructured.Unstructured) error
}

func New(kubeClient clients.ClientsInterface, lc lifecycle.Lifecycle, storage storage.Storage) PollActions {
	actions := pollActions{
		kubeClient: kubeClient,
		lc:         lc,
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
	err := p.kubeClient.Get(ctx, types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, found)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("%s/%s does not exists yet", obj.GetNamespace(), obj.GetName())
		}
		return fmt.Errorf("failed to get %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	return nil
}

func (p *pollActions) ForResourceUnavailability(ctx context.Context, obj *unstructured.Unstructured) error {

	found := obj.DeepCopy()
	err := p.kubeClient.Get(ctx, types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, found)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get %s/%s, not due to unfound error: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	return fmt.Errorf("resource %s/%s still exists", obj.GetNamespace(), obj.GetName())
}

// makeStatusCallback Closure capturing json path and expected status
func makeStatusCallback(status interface{}, fields ...string) statusCallback {
	_status := status
	_fields := fields
	return func(_ context.Context, obj *unstructured.Unstructured) (bool, error) {
		switch expected := _status.(type) {
		case int64:
			current, found, err := unstructured.NestedInt64(obj.Object, _fields...)
			if err != nil {
				return false, fmt.Errorf("error accessing unstructured object %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
			}
			if !found {
				return false, fmt.Errorf("int64 field %s in unstructured object %s/%s not found", strings.Join(_fields, ","), obj.GetNamespace(), obj.GetName())
			}

			return current == expected, nil

		case int:
			current, found, err := unstructured.NestedInt64(obj.Object, _fields...)
			if err != nil {
				return false, fmt.Errorf("error accessing unstructured object %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
			}
			if !found {
				return false, fmt.Errorf("int field %s in unstructured object %s/%s not found", strings.Join(_fields, ","), obj.GetNamespace(), obj.GetName())
			}

			return int(current) == expected, nil

		case string:
			current, found, err := unstructured.NestedString(obj.Object, _fields...)
			if err != nil {
				return false, fmt.Errorf("error accessing unstructured object %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
			}
			if !found {
				return false, fmt.Errorf("string field %s in unstructured object %s/%s not found", strings.Join(_fields, ","), obj.GetNamespace(), obj.GetName())
			}

			return current == expected, nil

		default:
			return false, fmt.Errorf("unhandled typre for status field: %T", _status)
		}
	}
}

func (p *pollActions) ForResource(ctx context.Context, obj *unstructured.Unstructured) error {

	var err error
	// Wait for general availability, Pods Complete, Running
	// DaemonSet NumberUnavailable == 0, etc
	if wait, ok := p.waitFor[obj.GetKind()]; ok {
		if err = wait(ctx, obj); err != nil {
			return fmt.Errorf("waiting too long for resource %s %s/%s: %w", obj.GetKind(), obj.GetNamespace(), obj.GetName(), err)
		}
	} else {
		ctrlruntime.LoggerFrom(ctx).Info(utils.WarnString("Missing wait function for the kind"), "kind", obj.GetKind())
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
		return fmt.Errorf("failed resource availablity for CRD %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}

	if _, err := p.kubeClient.ServerGroups(); err != nil {
		ctrlruntime.LoggerFrom(ctx).Info(utils.WarnString("failed to get ServerGroups for CRD"))
	}

	return nil
}

func (p *pollActions) forPod(ctx context.Context, obj *unstructured.Unstructured) error {
	if err := p.forResourceAvailability(ctx, obj); err != nil {
		return fmt.Errorf("failed resource availablity for Pod %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	callback := makeStatusCallback("Succeeded", "status", "phase")
	return p.forResourceFullAvailability(ctx, obj, callback)
}

func (p *pollActions) forDeployment(ctx context.Context, obj *unstructured.Unstructured) error {
	if err := p.forResourceAvailability(ctx, obj); err != nil {
		return fmt.Errorf("failed resource availablity for Deployment %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	return p.forResourceFullAvailability(ctx, obj, func(ctx context.Context, obj *unstructured.Unstructured) (bool, error) {
		labels, found, err := unstructured.NestedMap(obj.Object, "spec", "selector", "matchLabels")
		if err != nil {
			return false, fmt.Errorf("failed to obtain match labels from unstructured Deployment %s/%s: %w", obj.GetName(), obj.GetNamespace(), err)
		}

		if !found {
			return false, nil
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
			return false, fmt.Errorf("failed to list ReplicaSets: %w", err)
		}

		for _, rs := range rss.Items {
			status, found, err := unstructured.NestedMap(rs.Object, "status")
			if err != nil {
				return false, fmt.Errorf("failed to obtain status from unstructured ReplicaSet %s/%s: %w", rs.GetNamespace(), rs.GetName(), err)
			}
			if !found {
				return false, nil
			}

			_, ok := status["replicas"]
			if !ok {
				return false, nil
			}
			repls := status["replicas"].(int64)
			if repls == 0 {
				continue
			}
			_, okAvailableReplicas := status["availableReplicas"]
			if !okAvailableReplicas {
				return false, nil
			}
			avail := status["availableReplicas"].(int64)
			if avail != repls {
				return false, nil
			}
		}
		return true, nil
	})
}

func (p *pollActions) forStatefulSet(ctx context.Context, obj *unstructured.Unstructured) error {
	if err := p.forResourceAvailability(ctx, obj); err != nil {
		return fmt.Errorf("failed resource availablity for statefulset %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	return p.forResourceFullAvailability(ctx, obj, func(_ context.Context, obj *unstructured.Unstructured) (bool, error) {
		repls, found, err := unstructured.NestedInt64(obj.Object, "spec", "replicas")
		if err != nil {
			return false, fmt.Errorf("failed to obtain amount of replicas from unstructured StatefulSet  %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
		}
		if !found {
			return false, fmt.Errorf("missing .spec.replicas from StatefulSet %s/%s", obj.GetNamespace(), obj.GetName())
		}

		status, found, err := unstructured.NestedMap(obj.Object, "status")
		if err != nil {
			return false, fmt.Errorf("failed to obtain status from unstructured StatefulSet %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
		}
		if !found {
			return false, nil
		}
		if _, ok := status["currentReplicas"]; !ok {
			return false, nil
		}
		currt := status["currentReplicas"].(int64)
		if repls == currt {
			return true, nil
		}
		return false, nil
	})
}

func (p *pollActions) forJob(ctx context.Context, obj *unstructured.Unstructured) error {
	if err := p.forResourceAvailability(ctx, obj); err != nil {
		return fmt.Errorf("failed resource availablity for job %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}

	return p.forResourceFullAvailability(ctx, obj, func(_ context.Context, obj *unstructured.Unstructured) (bool, error) {
		conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
		if err != nil {
			return false, fmt.Errorf("failed to obtain conditions from unstructured Job %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
		}

		if !found {
			return false, nil
		}

		for _, condition := range conditions {

			status, found, err := unstructured.NestedString(condition.(map[string]interface{}), "status")
			if err != nil {
				return false, fmt.Errorf("failed to obtain condition status from job %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
			}
			if !found {
				return false, fmt.Errorf("status of the condition is not found in the job %s/%s", obj.GetNamespace(), obj.GetName())
			}

			if status == "True" {
				stype, found, err := unstructured.NestedString(condition.(map[string]interface{}), "type")
				if err != nil {
					return false, fmt.Errorf("failed to obtain type of the condition ofjob %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
				}
				if !found {
					return false, fmt.Errorf("type field of teh condition of the job %s/%s is not found", obj.GetNamespace(), obj.GetName())
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
	if err != nil {
		return false, fmt.Errorf("failed to get desiredNumberScheduled from status of DaemonSet %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	if !found {
		return false, fmt.Errorf("failed to find desiredNumberScheduled from status of DaemonSet %s/%s", obj.GetNamespace(), obj.GetName())
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
		return fmt.Errorf("failed to extract updatestrategy from DaemonSet %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
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

	pl := p.lc.GetPodFromDaemonSet(ctx, objKey)
	for _, pod := range pl.Items {
		podLabels := pod.GetLabels()
		podGenerator := podLabels[extensions.DaemonSetTemplateGenerationKey]
		if podGenerator != tempGenerator {
			return fmt.Errorf("old pod %s/%s is still running", obj.GetNamespace(), pod.GetName())
		}
	}

	return nil
}

func (p *pollActions) ForDaemonSet(ctx context.Context, obj *unstructured.Unstructured) error {
	if err := p.forResourceAvailability(ctx, obj); err != nil {
		return fmt.Errorf("DaemonSet %s/%s is not available, probably not fully created yet: %w", obj.GetNamespace(), obj.GetName(), err)
	}

	if err := p.forLifecycleAvailability(ctx, obj); err != nil {
		return fmt.Errorf("lifecycle availability of the DaemonSet %s/%s is not verified yet: %w", obj.GetNamespace(), obj.GetName(), err)
	}

	return p.forResourceFullAvailability(ctx, obj, p.forDaemonSetCallback)
}

func (p *pollActions) forBuild(ctx context.Context, obj *unstructured.Unstructured) error {

	if err := p.forResourceAvailability(ctx, obj); err != nil {
		return fmt.Errorf("failed resource availablity for Build %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}

	builds := &unstructured.UnstructuredList{}
	builds.SetAPIVersion("build.openshift.io/v1")
	builds.SetKind("build")

	opts := []client.ListOption{
		client.InNamespace(clients.Namespace),
	}
	if err := p.kubeClient.List(ctx, builds, opts...); err != nil {
		return fmt.Errorf("could not get BuildList: %w", err)
	}

	var build *unstructured.Unstructured
	for _, b := range builds.Items {
		slice, _, err := unstructured.NestedSlice(b.Object, "metadata", "ownerReferences")
		if err != nil {
			return fmt.Errorf("failed to get ownerreferences for BuildConfig %s/%s: %w", b.GetNamespace(), b.GetName(), err)
		}
		for _, element := range slice {
			if element.(map[string]interface{})["name"] == obj.GetName() {
				build = &b
				break
			}
		}
		if build != nil {
			break
		}
	}
	if build == nil {
		return fmt.Errorf("Build %s/%s object not yet available", obj.GetNamespace(), obj.GetName())
	}

	callback := makeStatusCallback("Complete", "status", "phase")
	return p.forResourceFullAvailability(ctx, build, callback)
}

func (p *pollActions) forResourceFullAvailability(ctx context.Context, obj *unstructured.Unstructured, callback statusCallback) error {

	found := obj.DeepCopy()
	err := p.kubeClient.Get(ctx, types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, found)
	if err != nil {
		return fmt.Errorf("failed to get object %s/%s/%s: %w", obj.GetKind(), obj.GetNamespace(), obj.GetName(), err)
	}
	fnd, err := callback(ctx, found)
	if err != nil {
		return fmt.Errorf("callback failed for %s/%s/%s: %w", obj.GetKind(), obj.GetNamespace(), obj.GetName(), err)
	}

	if fnd {
		return nil
	}
	return fmt.Errorf("resource %s/%s/%s not available", obj.GetKind(), obj.GetNamespace(), obj.GetName())
}

func (p *pollActions) ForDaemonSetLogs(ctx context.Context, obj *unstructured.Unstructured, pattern string) error {
	pods := &unstructured.UnstructuredList{}
	pods.SetAPIVersion("v1")
	pods.SetKind("pod")

	label := make(map[string]string)

	var found bool
	var selector string

	if selector, found = obj.GetLabels()["app"]; !found {
		return fmt.Errorf("cannot find Label app= for DaemonSet %s/%s, missing take a look at the manifests", obj.GetNamespace(), obj.GetName())
	}

	label["app"] = selector

	opts := []client.ListOption{
		client.InNamespace(clients.Namespace),
		client.MatchingLabels(label),
	}

	err := p.kubeClient.List(ctx, pods, opts...)
	if err != nil {
		return fmt.Errorf("could not get PodList of Daemonset %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}

	for _, pod := range pods.Items {
		podLogOpts := v1.PodLogOptions{}
		req := p.kubeClient.GetPodLogs(pod.GetNamespace(), pod.GetName(), &podLogOpts)
		podLogs, err := req.Stream(ctx)
		if err != nil {
			return fmt.Errorf("error in opening stream for pod %s/%s: %w", pod.GetNamespace(), pod.GetName(), err)
		}
		defer podLogs.Close()

		buf := new(bytes.Buffer)

		if _, err = io.Copy(buf, podLogs); err != nil {
			return fmt.Errorf("error in copy information from podLogs to buf for pod %s/%s: %w", pod.GetNamespace(), pod.GetName(), err)
		}

		cutoff := 100
		if buf.Len() <= 100 {
			cutoff = buf.Len()
		}

		logs := buf.String()
		lastBytes := logs[len(logs)-cutoff:]

		var match bool

		match, err = regexp.MatchString(pattern, lastBytes)
		if err != nil {
			return fmt.Errorf("error matching pattern %q: %w", pattern, err)
		}

		if !match {
			return fmt.Errorf("not yet done; not matched against %q", pattern)
		}
	}

	return nil
}
