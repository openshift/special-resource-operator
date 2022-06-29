package lifecycle

import (
	"context"

	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/warn"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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
