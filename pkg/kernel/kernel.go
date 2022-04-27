package kernel

import (
	"fmt"
	"strings"

	"github.com/openshift/special-resource-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate mockgen -source=kernel.go -package=kernel -destination=mock_kernel_api.go

type KernelData interface {
	SetAffineAttributes(obj *unstructured.Unstructured, kernelFullVersion, operatingSystemMajorMinor string) error
	IsObjectAffine(obj client.Object) bool
	FullVersion(*corev1.NodeList) (string, error)
	PatchVersion(kernelFullVersion string) (string, error)
}

type kernelData struct{}

func NewKernelData() KernelData {
	return &kernelData{}
}

func (k *kernelData) SetAffineAttributes(obj *unstructured.Unstructured,
	kernelFullVersion string,
	operatingSystemMajorMinor string) error {

	kernelVersion := strings.ReplaceAll(kernelFullVersion, "_", "-")
	hash64, err := utils.FNV64a(operatingSystemMajorMinor + "-" + kernelVersion)
	if err != nil {
		return fmt.Errorf("could not get hash: %w", err)
	}
	name := obj.GetName() + "-" + hash64
	obj.SetName(name)

	switch obj.GetKind() {
	case "BuildRun":
		if err := unstructured.SetNestedField(obj.Object, name, "spec", "buildRef", "name"); err != nil {
			return fmt.Errorf("could not set spec.buildRef.name in BuildRun object: %w", err)
		}
	case "DaemonSet", "Deployment", "StatefulSet":
		if err := unstructured.SetNestedField(obj.Object, name, "metadata", "labels", "app"); err != nil {
			return fmt.Errorf("could not set metadata.labels.app in DaemonSet object: %w", err)
		}

		if err := unstructured.SetNestedField(obj.Object, name, "spec", "selector", "matchLabels", "app"); err != nil {
			return fmt.Errorf("could not set spec.selector.matchLabels.app in DaemonSet object: %w", err)
		}

		if err := unstructured.SetNestedField(obj.Object, name, "spec", "template", "metadata", "labels", "app"); err != nil {
			return fmt.Errorf("could not set spec.template.metadata.labels.app in DaemonSet object: %w", err)
		}
	}

	if err := k.setVersionNodeAffinity(obj, kernelFullVersion); err != nil {
		return fmt.Errorf("cannot set kernel version node affinity for obj %s: %w", obj.GetKind(), err)
	}
	return nil
}

func (k *kernelData) setVersionNodeAffinity(obj *unstructured.Unstructured, kernelFullVersion string) error {

	switch obj.GetKind() {
	case "DaemonSet", "Deployment", "Statefulset":
		if err := k.versionNodeAffinity(kernelFullVersion, obj, "spec", "template", "spec", "nodeSelector"); err != nil {
			return fmt.Errorf("cannot setup %s's kernel version affinity: %w", obj.GetKind(), err)
		}
	case "Pod":
		if err := k.versionNodeAffinity(kernelFullVersion, obj, "spec", "nodeSelector"); err != nil {
			return fmt.Errorf("cannot setup %s's kernel version affinity: %w", obj.GetKind(), err)
		}
	case "BuildConfig":
		if err := k.versionNodeAffinity(kernelFullVersion, obj, "spec", "nodeSelector"); err != nil {
			return fmt.Errorf("cannot setup %s's kernel version affinity: %w", obj.GetKind(), err)
		}
	}
	return nil
}

func (k *kernelData) versionNodeAffinity(kernelFullVersion string, obj *unstructured.Unstructured, fields ...string) error {

	nodeSelector, found, err := unstructured.NestedMap(obj.Object, fields...)
	if err != nil {
		return fmt.Errorf("couldn't find %s in %s: %w", strings.Join(fields, "."), obj.GetKind(), err)
	}

	if !found {
		nodeSelector = make(map[string]interface{})
	}

	nodeSelector["feature.node.kubernetes.io/kernel-version.full"] = kernelFullVersion

	if err := unstructured.SetNestedMap(obj.Object, nodeSelector, fields...); err != nil {
		return fmt.Errorf("couldn't set %s in %s: %w", strings.Join(fields, "."), obj.GetKind(), err)
	}

	return nil
}

func (k *kernelData) IsObjectAffine(obj client.Object) bool {

	annotations := obj.GetAnnotations()

	affine, found := annotations["specialresource.openshift.io/kernel-affine"]
	return found && affine == "true"
}

func (k *kernelData) FullVersion(nodeList *corev1.NodeList) (string, error) {
	var kernelFullVersion string
	// Assuming all nodes are running the same kernel version,
	// one could easily add driver-kernel-versions for each node.
	for _, node := range nodeList.Items {
		kernelFullVersion = node.Status.NodeInfo.KernelVersion
		if len(kernelFullVersion) == 0 {
			return "", fmt.Errorf("kernel not found for node %s", node.Name)
		}
	}
	return kernelFullVersion, nil
}

// Using w.xx.y-zzz and looking at the fourth file listed /boot/vmlinuz-4.4.0-45 we can say:
// w = Kernel Version = 4
// xx= Major Revision = 4
// y = Minor Revision = 0
// zzz=Patch number = 45
func (k *kernelData) PatchVersion(kernelFullVersion string) (string, error) {

	version := strings.Split(kernelFullVersion, "-")
	// Happens only if kernel full version has no patch version sep by "-"
	if len(version) == 1 {
		short := strings.Split(kernelFullVersion, ".")
		return short[0] + "." + short[1] + "." + short[2], nil
	}

	patch := strings.Split(version[1], ".")
	// version.major.minor-patch
	return version[0] + "-" + patch[0], nil
}
