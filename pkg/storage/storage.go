package storage

import (
	"context"

	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/exit"
	"github.com/openshift-psap/special-resource-operator/pkg/warn"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

var Driver string

func init() {
	Driver = "ConfigMap"
}

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
	exit.OnError(err)

	if !found {
		return "", nil
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
	exit.OnError(err)

	if !found {
		entries = make(map[string]interface{})
	}

	entries[key] = value

	err = unstructured.SetNestedMap(cm.Object, entries, "data")
	exit.OnError(err)

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
	exit.OnError(err)

	if !found {
		return nil
	}

	new := make(map[string]interface{})

	for k, v := range old {
		if delete != k {
			new[k] = v
		}
	}

	err = unstructured.SetNestedMap(cm.Object, new, "data")
	exit.OnError(err)

	err = clients.Interface.Update(context.TODO(), cm)
	if err != nil {
		warn.OnError(err)
		return err
	}
	return nil
}
