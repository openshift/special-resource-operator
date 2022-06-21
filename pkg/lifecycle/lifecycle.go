package lifecycle

import (
	"context"
	"os"

	"github.com/go-logr/logr"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"github.com/openshift-psap/special-resource-operator/pkg/hash"
	"github.com/openshift-psap/special-resource-operator/pkg/storage"
	"github.com/openshift-psap/special-resource-operator/pkg/warn"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	log logr.Logger
)

func init() {
	log = zap.New(zap.UseDevMode(true)).WithName(color.Print("lifecycle", color.Green))
}

func GetPodFromDaemonSet(key types.NamespacedName) *v1.PodList {
	ds := &appsv1.DaemonSet{}

	err := clients.Interface.Get(context.TODO(), key, ds)
	if apierrors.IsNotFound(err) || err != nil {
		warn.OnError(err)
		return &v1.PodList{}
	}

	return getPodListForUpperObject(ds.Spec.Selector.MatchLabels, key.Namespace)
}

func GetPodFromDeployment(key types.NamespacedName) *v1.PodList {
	dp := &appsv1.Deployment{}

	err := clients.Interface.Get(context.TODO(), key, dp)
	if apierrors.IsNotFound(err) || err != nil {
		warn.OnError(err)
		return &v1.PodList{}
	}

	return getPodListForUpperObject(dp.Spec.Selector.MatchLabels, key.Namespace)
}

func getPodListForUpperObject(matchLabels map[string]string, ns string) *v1.PodList {
	pl := &v1.PodList{}

	opts := []client.ListOption{
		client.InNamespace(ns),
		client.MatchingLabels(matchLabels),
	}

	if err := clients.Interface.List(context.TODO(), pl, opts...); err != nil {
		warn.OnError(err)
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
