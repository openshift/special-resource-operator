package specialresource

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type controlFunc []func(n SRO) (ResourceStatus, error)

type ResourceStatus int

const (
	Ready    ResourceStatus = 0
	NotReady ResourceStatus = 1
)

func (s ResourceStatus) String() string {
	names := [...]string{
		"Ready",
		"NotReady"}

	if s < Ready || s > NotReady {
		return "Unkown Resources Status"
	}
	return names[s]
}

func ServiceAccount(n SRO) (ResourceStatus, error) {

	state := n.idx
	obj := &n.resources[state].ServiceAccount

	found := &corev1.ServiceAccount{}
	logger := log.WithValues("ServiceAccount", obj.Name, "Namespace", obj.Namespace)

	if err := controllerutil.SetControllerReference(n.ins, obj, n.rec.scheme); err != nil {
		return NotReady, err
	}

	logger.Info("Looking for")
	err := n.rec.client.Get(context.TODO(), types.NamespacedName{Namespace: obj.Namespace, Name: obj.Name}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Not found, creating")
		err = n.rec.client.Create(context.TODO(), obj)
		if err != nil {
			logger.Info("Couldn't create")
			return NotReady, err
		}
		return Ready, nil
	} else if err != nil {
		return NotReady, err
	}

	logger.Info("Found")

	return Ready, nil
}

func Role(n SRO) (ResourceStatus, error) {

	state := n.idx
	obj := &n.resources[state].Role

	found := &rbacv1.Role{}
	logger := log.WithValues("Role", obj.Name, "Namespace", obj.Namespace)

	if err := controllerutil.SetControllerReference(n.ins, obj, n.rec.scheme); err != nil {
		return NotReady, err
	}

	logger.Info("Looking for")
	err := n.rec.client.Get(context.TODO(), types.NamespacedName{Namespace: obj.Namespace, Name: obj.Name}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Not found, creating")
		err = n.rec.client.Create(context.TODO(), obj)
		if err != nil {
			logger.Info("Couldn't create")
			return NotReady, err
		}
		return Ready, nil
	} else if err != nil {
		return NotReady, err
	}

	logger.Info("Found")

	return Ready, nil
}

func RoleBinding(n SRO) (ResourceStatus, error) {

	state := n.idx
	obj := &n.resources[state].RoleBinding

	found := &rbacv1.RoleBinding{}
	logger := log.WithValues("RoleBinding", obj.Name, "Namespace", obj.Namespace)

	if err := controllerutil.SetControllerReference(n.ins, obj, n.rec.scheme); err != nil {
		return NotReady, err
	}

	logger.Info("Looking for")
	err := n.rec.client.Get(context.TODO(), types.NamespacedName{Namespace: obj.Namespace, Name: obj.Name}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Not found, creating")
		err = n.rec.client.Create(context.TODO(), obj)
		if err != nil {
			logger.Info("Couldn't create")
			return NotReady, err
		}
		return Ready, nil
	} else if err != nil {
		return NotReady, err
	}

	logger.Info("Found")

	return Ready, nil
}

func ClusterRole(n SRO) (ResourceStatus, error) {

	state := n.idx
	obj := &n.resources[state].ClusterRole

	found := &rbacv1.ClusterRole{}
	logger := log.WithValues("ClusterRole", obj.Name, "Namespace", obj.Namespace)

	if err := controllerutil.SetControllerReference(n.ins, obj, n.rec.scheme); err != nil {
		return NotReady, err
	}

	logger.Info("Looking for")
	err := n.rec.client.Get(context.TODO(), types.NamespacedName{Namespace: "", Name: obj.Name}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Not found, creating")
		err = n.rec.client.Create(context.TODO(), obj)
		if err != nil {
			logger.Info("Couldn't create")
			return NotReady, err
		}
		return Ready, nil
	} else if err != nil {
		return NotReady, err
	}

	logger.Info("Found")

	return Ready, nil
}

func ClusterRoleBinding(n SRO) (ResourceStatus, error) {

	state := n.idx
	obj := &n.resources[state].ClusterRoleBinding

	found := &rbacv1.ClusterRoleBinding{}
	logger := log.WithValues("ClusterRoleBinding", obj.Name, "Namespace", obj.Namespace)

	if err := controllerutil.SetControllerReference(n.ins, obj, n.rec.scheme); err != nil {
		return NotReady, err
	}

	logger.Info("Looking for")
	err := n.rec.client.Get(context.TODO(), types.NamespacedName{Namespace: "", Name: obj.Name}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Not found, creating")
		err = n.rec.client.Create(context.TODO(), obj)
		if err != nil {
			logger.Info("Couldn't create")
			return NotReady, err
		}
		return Ready, nil
	} else if err != nil {
		return NotReady, err
	}

	logger.Info("Found")

	return Ready, nil
}

func ConfigMap(n SRO) (ResourceStatus, error) {

	state := n.idx
	obj := &n.resources[state].ConfigMap

	found := &corev1.ConfigMap{}
	logger := log.WithValues("ConfigMap", obj.Name, "Namespace", obj.Namespace)

	if err := controllerutil.SetControllerReference(n.ins, obj, n.rec.scheme); err != nil {
		return NotReady, err
	}

	logger.Info("Looking for")
	err := n.rec.client.Get(context.TODO(), types.NamespacedName{Namespace: obj.Namespace, Name: obj.Name}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Not found, creating")
		err = n.rec.client.Create(context.TODO(), obj)
		if err != nil {
			logger.Info("Couldn't create")
			return NotReady, err
		}
		return Ready, nil
	} else if err != nil {
		return NotReady, err
	}

	logger.Info("Found")

	return Ready, nil
}

func kernelFullVersion(n SRO) string {

	logger := log.WithValues("Request.Namespace", "default", "Request.Name", "Node")
	// We need the node labels to fetch the correct container
	opts := &client.ListOptions{}
	opts.SetLabelSelector("feature.node.kubernetes.io/pci-0300_10de.present=true")
	list := &corev1.NodeList{}
	err := n.rec.client.List(context.TODO(), opts, list)
	if err != nil {
		logger.Info("Could not get NodeList", err)
	}
	// Assuming all nodes are running the same kernel version,
	// One could easily add driver-kernel-versions for each node.
	node := list.Items[0]
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

func isDaemonSetReady(d *appsv1.DaemonSet, n SRO) ResourceStatus {

	opts := &client.ListOptions{}
	opts.SetLabelSelector(fmt.Sprintf("app=%s", d.Name))
	log.Info("#### DaemonSet", "LabelSelector", fmt.Sprintf("app=%s", d.Name))
	list := &appsv1.DaemonSetList{}
	err := n.rec.client.List(context.TODO(), opts, list)
	if err != nil {
		log.Info("Could not get DaemonSetList", err)
	}
	log.Info("#### DaemonSet", "NumberOfDaemonSets", len(list.Items))
	if len(list.Items) == 0 {
		return NotReady
	}

	ds := list.Items[0]
	log.Info("#### DaemonSet", "NumberUnavailable", ds.Status.NumberUnavailable)

	if ds.Status.NumberUnavailable != 0 {
		return NotReady
	}
	return Ready
}

func DaemonSet(n SRO) (ResourceStatus, error) {

	state := n.idx
	obj := &n.resources[state].DaemonSet

	preProcessDaemonSet(obj, n)

	found := &appsv1.DaemonSet{}
	logger := log.WithValues("DaemonSet", obj.Name, "Namespace", obj.Namespace)

	if err := controllerutil.SetControllerReference(n.ins, obj, n.rec.scheme); err != nil {
		return NotReady, err
	}

	logger.Info("Looking for")
	err := n.rec.client.Get(context.TODO(), types.NamespacedName{Namespace: obj.Namespace, Name: obj.Name}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Not found, creating")
		err = n.rec.client.Create(context.TODO(), obj)
		if err != nil {
			logger.Info("Couldn't create")
			return NotReady, err
		}
		return isDaemonSetReady(obj, n), nil
	} else if err != nil {
		return NotReady, err
	}

	logger.Info("Found")

	return isDaemonSetReady(obj, n), nil
}

// The operator starts two pods in different stages to validate
// the correct working of the DaemonSets (driver and dp). Therefore
// the operator waits until the Pod completes and checks the error status
// to advance to the next state.
func isPodReady(d *corev1.Pod, n SRO) ResourceStatus {
	opts := &client.ListOptions{}
	opts.SetLabelSelector(fmt.Sprintf("app=%s", d.Name))
	log.Info("#### Pod", "LabelSelector", fmt.Sprintf("app=%s", d.Name))
	list := &corev1.PodList{}
	err := n.rec.client.List(context.TODO(), opts, list)
	if err != nil {
		log.Info("Could not get PodList", err)
	}
	log.Info("#### Pod", "NumberOfPods", len(list.Items))
	if len(list.Items) == 0 {
		return NotReady
	}

	pd := list.Items[0]
	log.Info("#### Pod", "Phase", pd.Status.Phase)

	// if ds.Status.NumberUnavailable != 0 {
	// 	return NotReady
	// }
	return Ready
}

func Pod(n SRO) (ResourceStatus, error) {

	state := n.idx
	obj := &n.resources[state].Pod

	found := &corev1.Pod{}
	logger := log.WithValues("Pod", obj.Name, "Namespace", obj.Namespace)

	if err := controllerutil.SetControllerReference(n.ins, obj, n.rec.scheme); err != nil {
		return NotReady, err
	}

	logger.Info("Looking for")
	err := n.rec.client.Get(context.TODO(), types.NamespacedName{Namespace: obj.Namespace, Name: obj.Name}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Not found, creating")
		err = n.rec.client.Create(context.TODO(), obj)
		if err != nil {
			logger.Info("Couldn't create")
			return NotReady, err
		}
		return Ready, nil
	} else if err != nil {
		return NotReady, err
	}

	logger.Info("Found")

	return isPodReady(obj, n), nil
}

func Service(n SRO) (ResourceStatus, error) {

	state := n.idx
	obj := &n.resources[state].Service

	found := &corev1.Service{}
	logger := log.WithValues("Service", obj.Name, "Namespace", obj.Namespace)

	if err := controllerutil.SetControllerReference(n.ins, obj, n.rec.scheme); err != nil {
		return NotReady, err
	}

	logger.Info("Looking for")
	err := n.rec.client.Get(context.TODO(), types.NamespacedName{Namespace: obj.Namespace, Name: obj.Name}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Not found, creating")
		err = n.rec.client.Create(context.TODO(), obj)
		if err != nil {
			logger.Info("Couldn't create")
			return NotReady, err
		}
		return Ready, nil
	} else if err != nil {
		return NotReady, err
	}

	logger.Info("Found")

	return Ready, nil
}
