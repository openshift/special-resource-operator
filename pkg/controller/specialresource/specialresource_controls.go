package specialresource

import (
	"context"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type controlFunc []func(n SRO) error

func ServiceAccount(n SRO) error {

	state := n.idx
	obj := &n.resources[state].ServiceAccount

	found := &corev1.ServiceAccount{}
	logger := log.WithValues("ServiceAccount", obj.Name, "Namespace", obj.Namespace)

	logger.Info("Looking for")
	err := n.rec.client.Get(context.TODO(), types.NamespacedName{Namespace: obj.Namespace, Name: obj.Name}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Not found, creating")
		err = n.rec.client.Create(context.TODO(), obj)
		if err != nil {
			logger.Info("Couldn't create")
			return err
		}
		return nil
	} else if err != nil {
		return err
	}

	logger.Info("Found")

	return nil
}
func ClusterRole(n SRO) error {

	state := n.idx
	obj := &n.resources[state].ClusterRole

	found := &rbacv1.ClusterRole{}
	logger := log.WithValues("ClusterRole", obj.Name, "Namespace", obj.Namespace)

	logger.Info("Looking for")
	err := n.rec.client.Get(context.TODO(), types.NamespacedName{Namespace: "", Name: obj.Name}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Not found, creating")
		err = n.rec.client.Create(context.TODO(), obj)
		if err != nil {
			logger.Info("Couldn't create")
			return err
		}
		return nil
	} else if err != nil {
		return err
	}

	logger.Info("Found")

	return nil
}

func Role(n SRO) error {

	state := n.idx
	obj := &n.resources[state].Role

	found := &rbacv1.Role{}
	logger := log.WithValues("Role", obj.Name, "Namespace", obj.Namespace)

	logger.Info("Looking for")
	err := n.rec.client.Get(context.TODO(), types.NamespacedName{Namespace: "", Name: obj.Name}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Not found, creating")
		err = n.rec.client.Create(context.TODO(), obj)
		if err != nil {
			logger.Info("Couldn't create")
			return err
		}
		return nil
	} else if err != nil {
		return err
	}

	logger.Info("Found")

	return nil
}

func ClusterRoleBinding(n SRO) error {

	state := n.idx
	obj := &n.resources[state].ClusterRoleBinding

	found := &rbacv1.ClusterRoleBinding{}
	logger := log.WithValues("ClusterRoleBinding", obj.Name, "Namespace", obj.Namespace)

	logger.Info("Looking for")
	err := n.rec.client.Get(context.TODO(), types.NamespacedName{Namespace: "", Name: obj.Name}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Not found, creating")
		err = n.rec.client.Create(context.TODO(), obj)
		if err != nil {
			logger.Info("Couldn't create")
			return err
		}
		return nil
	} else if err != nil {
		return err
	}

	logger.Info("Found")

	return nil
}

func RoleBinding(n SRO) error {

	state := n.idx
	obj := &n.resources[state].RoleBinding

	found := &rbacv1.RoleBinding{}
	logger := log.WithValues("RoleBinding", obj.Name, "Namespace", obj.Namespace)

	logger.Info("Looking for")
	err := n.rec.client.Get(context.TODO(), types.NamespacedName{Namespace: "", Name: obj.Name}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Not found, creating")
		err = n.rec.client.Create(context.TODO(), obj)
		if err != nil {
			logger.Info("Couldn't create")
			return err
		}
		return nil
	} else if err != nil {
		return err
	}

	logger.Info("Found")

	return nil
}

func ConfigMap(n SRO) error {

	state := n.idx
	obj := &n.resources[state].ConfigMap

	found := &corev1.ConfigMap{}
	logger := log.WithValues("ConfigMap", obj.Name, "Namespace", obj.Namespace)
	logger.Info("Looking for")
	err := n.rec.client.Get(context.TODO(), types.NamespacedName{Namespace: obj.Namespace, Name: obj.Name}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Not found, creating")
		err = n.rec.client.Create(context.TODO(), obj)
		if err != nil {
			logger.Info("Couldn't create")
			return err
		}
		return nil
	} else if err != nil {
		return err
	}

	logger.Info("Found")

	return nil
}

func kernelFullVersion(n SRO) string {

	logger := log.WithValues("Request.Namespace", "default", "Request.Name", "Node")
	// We need the node labels to fetch the correct container
	opts := &client.ListOptions{}
	opts.SetLabelSelector("feature.node.kubernetes.io/pci-0300_10de.present=true")
	nodelist := &corev1.NodeList{}
	err := n.rec.client.List(context.TODO(), opts, nodelist)
	if err != nil {
		logger.Info("Could not get NodeList", err)
	}

	node := nodelist.Items[0]
	labels := node.GetLabels()

	var ok bool
	kernelFullVersion, ok := labels["feature.node.kubernetes.io/kernel-version.full"]
	if ok {
		logger.Info(kernelFullVersion)
	} else {
		err := errors.NewNotFound(schema.GroupResource{Group: "Node", Resource: "Label"},
			"feature.node.kubernetes.io/kernel-version.full")
		logger.Info("Couldn't get kernelVersion", err)
		return ""
	}
	return kernelFullVersion
}

func preProcessDaemonSet(obj *appsv1.DaemonSet, n SRO) {
	if obj.Name == "nvidia-driver-daemonset" {
		kernelVersion := kernelFullVersion(n)
		img := obj.Spec.Template.Spec.Containers[0].Image
		img = strings.Replace(img, "KERNEL_FULL_VERSION", kernelVersion, -1)
		obj.Spec.Template.Spec.Containers[0].Image = img
		sel := "feature.node.kubernetes.io/kernel-version.full"
		obj.Spec.Template.Spec.NodeSelector[sel] = kernelVersion
	}
}

func DaemonSet(n SRO) error {

	state := n.idx
	obj := &n.resources[state].DaemonSet

	preProcessDaemonSet(obj, n)

	found := &appsv1.DaemonSet{}
	logger := log.WithValues("DaemonSet", obj.Name, "Namespace", obj.Namespace)

	logger.Info("Looking for")
	err := n.rec.client.Get(context.TODO(), types.NamespacedName{Namespace: obj.Namespace, Name: obj.Name}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Not found, creating")
		err = n.rec.client.Create(context.TODO(), obj)
		if err != nil {
			logger.Info("Couldn't create")
			return err
		}
		return nil
	} else if err != nil {
		return err
	}

	logger.Info("Found")

	return nil
}

func Service(n SRO) error {

	state := n.idx
	obj := &n.resources[state].Service

	found := &corev1.Service{}
	logger := log.WithValues("Service", obj.Name, "Namespace", obj.Namespace)

	logger.Info("Looking for")
	err := n.rec.client.Get(context.TODO(), types.NamespacedName{Namespace: obj.Namespace, Name: obj.Name}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Not found, creating")
		err = n.rec.client.Create(context.TODO(), obj)
		if err != nil {
			logger.Info("Couldn't create")
			return err
		}
		return nil
	} else if err != nil {
		return err
	}

	logger.Info("Found")

	return nil
}
