package lifecycle

import (
	"context"
	"os"

	"github.com/go-logr/logr"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/storage"
	"github.com/openshift-psap/special-resource-operator/pkg/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

//go:generate mockgen -source=lifecycle.go -package=lifecycle -destination=mock_lifecycle_api.go

type Lifecycle interface {
	GetPodFromDaemonSet(context.Context, types.NamespacedName) unstructured.UnstructuredList
	GetPodFromDeployment(context.Context, types.NamespacedName) unstructured.UnstructuredList
	UpdateDaemonSetPods(context.Context, client.Object) error
}

type lifecycle struct {
	kubeClient clients.ClientsInterface
	log        logr.Logger
	storage    storage.Storage
}

func New(kubeClient clients.ClientsInterface, storage storage.Storage) Lifecycle {
	return &lifecycle{
		kubeClient: kubeClient,
		log:        zap.New(zap.UseDevMode(true)).WithName(utils.Print("lifecycle", utils.Green)),
		storage:    storage,
	}
}

func (l *lifecycle) GetPodFromDaemonSet(ctx context.Context, key types.NamespacedName) unstructured.UnstructuredList {
	ds := &unstructured.Unstructured{}
	ds.SetAPIVersion("apps/v1")
	ds.SetKind("DaemonSet")

	err := l.kubeClient.Get(ctx, key, ds)
	if apierrors.IsNotFound(err) || err != nil {
		utils.WarnOnError(err)
		return unstructured.UnstructuredList{}
	}

	return l.getPodListForUpperObject(ctx, ds)
}

func (l *lifecycle) GetPodFromDeployment(ctx context.Context, key types.NamespacedName) unstructured.UnstructuredList {

	dp := &unstructured.Unstructured{}
	dp.SetAPIVersion("apps/v1")
	dp.SetKind("Deployment")

	err := l.kubeClient.Get(ctx, key, dp)
	if apierrors.IsNotFound(err) || err != nil {
		utils.WarnOnError(err)
		return unstructured.UnstructuredList{}
	}

	return l.getPodListForUpperObject(ctx, dp)
}

func (l *lifecycle) getPodListForUpperObject(ctx context.Context, obj *unstructured.Unstructured) unstructured.UnstructuredList {
	pl := unstructured.UnstructuredList{}
	pl.SetKind("PodList")
	pl.SetAPIVersion("v1")

	labels, found, err := unstructured.NestedMap(obj.Object, "spec", "selector", "matchLabels")
	if err != nil || !found {
		utils.WarnOnError(err)
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

	err = l.kubeClient.List(ctx, &pl, opts...)
	if err != nil {
		utils.WarnOnError(err)
		return pl
	}

	return pl
}

func (l *lifecycle) UpdateDaemonSetPods(ctx context.Context, obj client.Object) error {

	l.log.Info("UpdateDaemonSetPods")

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
		l.log.Info(pod.GetName(), "hs", hs, "value", value)
		err = l.storage.UpdateConfigMapEntry(ctx, hs, value, ins)
		if err != nil {
			utils.WarnOnError(err)
			return err
		}
	}

	return nil
}
