package lifecycle

import (
	"context"
	"fmt"
	"os"

	"github.com/openshift/special-resource-operator/pkg/clients"
	"github.com/openshift/special-resource-operator/pkg/storage"
	"github.com/openshift/special-resource-operator/pkg/utils"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate mockgen -source=lifecycle.go -package=lifecycle -destination=mock_lifecycle_api.go

type Lifecycle interface {
	GetPodFromDaemonSet(context.Context, types.NamespacedName) *v1.PodList
	GetPodFromDeployment(context.Context, types.NamespacedName) *v1.PodList
	UpdateDaemonSetPods(context.Context, client.Object) error
}

type lifecycle struct {
	kubeClient clients.ClientsInterface
	storage    storage.Storage
}

func New(kubeClient clients.ClientsInterface, storage storage.Storage) Lifecycle {
	return &lifecycle{
		kubeClient: kubeClient,
		storage:    storage,
	}
}

func (l *lifecycle) GetPodFromDaemonSet(ctx context.Context, key types.NamespacedName) *v1.PodList {
	ds := &appsv1.DaemonSet{}

	err := l.kubeClient.Get(ctx, key, ds)
	if apierrors.IsNotFound(err) || err != nil {
		if err != nil {
			ctrl.LoggerFrom(ctx).Info(utils.WarnString("Failed to get DaemonSet"), "key", key, "error", err)
		}
		return &v1.PodList{}
	}

	return l.getPodListForUpperObject(ctx, ds.Spec.Selector.MatchLabels, key.Namespace)
}

func (l *lifecycle) GetPodFromDeployment(ctx context.Context, key types.NamespacedName) *v1.PodList {
	dp := &appsv1.Deployment{}

	err := l.kubeClient.Get(ctx, key, dp)
	if apierrors.IsNotFound(err) || err != nil {
		if err != nil {
			ctrl.LoggerFrom(ctx).Info(utils.WarnString("Failed to get Deployment"), "key", key, "error", err)
		}
		return &v1.PodList{}
	}

	return l.getPodListForUpperObject(ctx, dp.Spec.Selector.MatchLabels, key.Namespace)
}

func (l *lifecycle) getPodListForUpperObject(ctx context.Context, matchLabels map[string]string, ns string) *v1.PodList {
	pl := &v1.PodList{}

	opts := []client.ListOption{
		client.InNamespace(ns),
		client.MatchingLabels(matchLabels),
	}

	if err := l.kubeClient.List(ctx, pl, opts...); err != nil {
		ctrl.LoggerFrom(ctx).Info(utils.WarnString("Failed to list Pods"), "ns", ns, "labels", matchLabels, "error", err)
	}

	return pl
}

func (l *lifecycle) UpdateDaemonSetPods(ctx context.Context, obj client.Object) error {
	log := ctrl.LoggerFrom(ctx)

	log.Info("UpdateDaemonSetPods")

	key := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
	ins := types.NamespacedName{
		Namespace: os.Getenv("OPERATOR_NAMESPACE"),
		Name:      "special-resource-lifecycle",
	}

	pl := l.GetPodFromDaemonSet(ctx, key)

	for _, pod := range pl.Items {

		hs, err := utils.FNV64a(pod.GetNamespace() + pod.GetName())
		if err != nil {
			return err
		}
		value := "*v1.Pod"
		log.Info(pod.GetName(), "hs", hs, "value", value)
		err = l.storage.UpdateConfigMapEntry(ctx, hs, value, ins)
		if err != nil {
			return fmt.Errorf("Failed to update '%s:%s' in ConfigMap %v: %w", hs, value, ins, err)
		}
	}

	return nil
}
