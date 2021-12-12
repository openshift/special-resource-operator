package lifecycle

import (
	"context"
	"os"

	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"github.com/openshift-psap/special-resource-operator/pkg/hash"
	"github.com/openshift-psap/special-resource-operator/pkg/storage"
	"github.com/openshift-psap/special-resource-operator/pkg/warn"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var log = zap.New(zap.UseDevMode(true)).WithName(color.Print("lifecycle", color.Green))

func GetPodFromDaemonSet(key types.NamespacedName) unstructured.UnstructuredList {
	ds := &unstructured.Unstructured{}
	ds.SetAPIVersion("apps/v1")
	ds.SetKind("DaemonSet")

	err := clients.Interface.Get(context.TODO(), key, ds)
	if apierrors.IsNotFound(err) || err != nil {
		warn.OnError(err)
		return unstructured.UnstructuredList{}
	}

	return getPodListForUpperObject(ds)
}

func GetPodFromDeployment(key types.NamespacedName) unstructured.UnstructuredList {

	dp := &unstructured.Unstructured{}
	dp.SetAPIVersion("apps/v1")
	dp.SetKind("Deployment")

	err := clients.Interface.Get(context.TODO(), key, dp)
	if apierrors.IsNotFound(err) || err != nil {
		warn.OnError(err)
		return unstructured.UnstructuredList{}
	}

	return getPodListForUpperObject(dp)
}

func getPodListForUpperObject(obj *unstructured.Unstructured) unstructured.UnstructuredList {
	pl := unstructured.UnstructuredList{}
	pl.SetKind("PodList")
	pl.SetAPIVersion("v1")

	labels, found, err := unstructured.NestedMap(obj.Object, "spec", "selector", "matchLabels")
	if err != nil || !found {
		warn.OnError(err)
		return pl
	}

	matchLabels := make(map[string]string)
	for k, v := range labels {
		matchLabels[k] = v.(string)
	}

	opts := []client.ListOption{
		client.InNamespace(obj.GetNamespace()),
		client.MatchingLabels(matchLabels),
	}

	err = clients.Interface.List(context.TODO(), &pl, opts...)
	if err != nil {
		warn.OnError(err)
		return pl
	}

	return pl
}

func UpdateDaemonSetPods(obj client.Object) error {

	log.Info("UpdateDaemonSetPods")

	key := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
	ins := types.NamespacedName{
		Namespace: os.Getenv("OPERATOR_NAMESPACE"),
		Name:      "special-resource-lifecycle",
	}

	pl := GetPodFromDaemonSet(key)

	for _, pod := range pl.Items {

		hs, err := hash.FNV64a(pod.GetNamespace() + pod.GetName())
		if err != nil {
			return err
		}
		value := "*v1.Pod"
		log.Info(pod.GetName(), "hs", hs, "value", value)
		err = storage.UpdateConfigMapEntry(hs, value, ins)
		if err != nil {
			warn.OnError(err)
			return err
		}
	}

	return nil
}
