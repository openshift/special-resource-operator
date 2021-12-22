package storage

import (
	"context"

	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/warn"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate mockgen -source=storage.go -package=storage -destination=mock_storage_api.go

type Storage interface {
	CheckConfigMapEntry(string, types.NamespacedName) (string, error)
	UpdateConfigMapEntry(string, string, types.NamespacedName) error
	DeleteConfigMapEntry(string, types.NamespacedName) error
}

type storage struct {
	kubeClient clients.ClientsInterface
}

func NewStorage(kubeClient clients.ClientsInterface) Storage {
	return &storage{kubeClient: kubeClient}
}

func (s *storage) CheckConfigMapEntry(key string, ins types.NamespacedName) (string, error) {
	cm, err := s.getConfigMap(ins.Namespace, ins.Name)
	if err != nil {
		return "", err
	}

	return cm.Data[key], nil
}

func (s *storage) UpdateConfigMapEntry(key string, value string, ins types.NamespacedName) error {
	cm, err := s.getConfigMap(ins.Namespace, ins.Name)
	if err != nil {
		warn.OnError(err)
		return err
	}

	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}

	if cm.Data[key] != value {
		cm.Data[key] = value

		if err = s.updateObject(context.TODO(), cm); err != nil {
			warn.OnError(err)
			return err
		}
	}

	return nil
}

func (s *storage) DeleteConfigMapEntry(key string, ins types.NamespacedName) error {
	cm, err := s.getConfigMap(ins.Namespace, ins.Name)
	if err != nil {
		warn.OnError(err)
		return err
	}

	if _, ok := cm.Data[key]; ok {
		delete(cm.Data, key)

		if err = s.updateObject(context.TODO(), cm); err != nil {
			warn.OnError(err)
			return err
		}
	}

	return nil
}

func (s *storage) getConfigMap(namespace string, name string) (*v1.ConfigMap, error) {
	cm := &v1.ConfigMap{}
	dep := types.NamespacedName{Namespace: namespace, Name: name}

	err := s.kubeClient.Get(context.TODO(), dep, cm)

	if apierrors.IsNotFound(err) {
		warn.OnError(err)
		return nil, err
	}

	return cm, err
}

func (s *storage) updateObject(ctx context.Context, cm client.Object) error {
	return s.kubeClient.Update(ctx, cm)
}
