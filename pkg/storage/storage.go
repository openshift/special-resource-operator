package storage

import (
	"context"

	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/warn"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

func GetConfigMap(namespace string, name string) (*unstructured.Unstructured, error) {

	cm := &unstructured.Unstructured{}
	cm.SetAPIVersion("v1")
	cm.SetKind("ConfigMap")

	dep := types.NamespacedName{Namespace: namespace, Name: name}

	err := clients.Interface.Get(context.TODO(), dep, cm)

	if apierrors.IsNotFound(err) {
		warn.OnError(err)
		return cm, err
	}

	return cm, err
}

func CheckConfigMapEntry(key string, ins types.NamespacedName) (string, error) {

	cm, err := GetConfigMap(ins.Namespace, ins.Name)
	if err != nil {
		return "", err
	}

	data, found, err := unstructured.NestedMap(cm.Object, "data")
	if err != nil || !found {
		return "", err
	}

	if value, found := data[key]; found {
		return value.(string), nil
	}

	return "", nil
}

func UpdateConfigMapEntry(key string, value string, ins types.NamespacedName) error {

	cm, err := GetConfigMap(ins.Namespace, ins.Name)
	if err != nil {
		warn.OnError(err)
		return err
	}

	// Just looking if exists so we can create or update
	entries, found, err := unstructured.NestedMap(cm.Object, "data")
	if err != nil {
		return err
	}

	if !found {
		entries = make(map[string]interface{})
	}

	entries[key] = value

	if err = unstructured.SetNestedMap(cm.Object, entries, "data"); err != nil {
		return err
	}

	err = clients.Interface.Update(context.TODO(), cm)
	if err != nil {
		warn.OnError(err)
		return err
	}
	return nil
}

func DeleteConfigMapEntry(delete string, ins types.NamespacedName) error {

	cm, err := GetConfigMap(ins.Namespace, ins.Name)
	if err != nil {
		warn.OnError(err)
		return err
	}

	// Just looking if exists so we can create or update
	old, found, err := unstructured.NestedMap(cm.Object, "data")
	if err != nil {
		return err
	}

	if !found {
		return nil
	}

	newMap := make(map[string]interface{})

	for k, v := range old {
		if delete != k {
			newMap[k] = v
		}
	}

	if err = unstructured.SetNestedMap(cm.Object, newMap, "data"); err != nil {
		return err
	}

	err = clients.Interface.Update(context.TODO(), cm)
	if err != nil {
		warn.OnError(err)
		return err
	}
	return nil
}
