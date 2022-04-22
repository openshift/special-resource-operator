package kernel

import (
	"strings"

	"github.com/openshift-psap/special-resource-operator/pkg/utils"
	"github.com/pkg/errors"
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
		return err
	}
	name := obj.GetName() + "-" + hash64
	obj.SetName(name)

	if obj.GetKind() == "BuildRun" {
		if err = unstructured.SetNestedField(obj.Object, name, "spec", "buildRef", "name"); err != nil {
			return err
		}
	}

	if obj.GetKind() == "DaemonSet" || obj.GetKind() == "Deployment" || obj.GetKind() == "StatefulSet" {
		if err = unstructured.SetNestedField(obj.Object, name, "metadata", "labels", "app"); err != nil {
			return err
		}

		if err = unstructured.SetNestedField(obj.Object, name, "spec", "selector", "matchLabels", "app"); err != nil {
			return err
		}

		if err = unstructured.SetNestedField(obj.Object, name, "spec", "template", "metadata", "labels", "app"); err != nil {
			return err
		}

		if err = unstructured.SetNestedField(obj.Object, name, "spec", "template", "metadata", "labels", "app"); err != nil {
			return err
		}
	}

	if err := k.setVersionNodeAffinity(obj, kernelFullVersion); err != nil {
		return errors.Wrap(err, "Cannot set kernel version node affinity for obj: "+obj.GetKind())
	}
	return nil
}

func (k *kernelData) setVersionNodeAffinity(obj *unstructured.Unstructured, kernelFullVersion string) error {

	if strings.Compare(obj.GetKind(), "DaemonSet") == 0 ||
		strings.Compare(obj.GetKind(), "Deployment") == 0 ||
		strings.Compare(obj.GetKind(), "Statefulset") == 0 {
		if err := k.versionNodeAffinity(kernelFullVersion, obj, "spec", "template", "spec", "nodeSelector"); err != nil {
			return errors.Wrap(err, "Cannot setup DaemonSet kernel version affinity")
		}
	}
	if strings.Compare(obj.GetKind(), "Pod") == 0 {
		if err := k.versionNodeAffinity(kernelFullVersion, obj, "spec", "nodeSelector"); err != nil {
			return errors.Wrap(err, "Cannot setup Pod kernel version affinity")
		}
	}
	if strings.Compare(obj.GetKind(), "BuildConfig") == 0 {
		if err := k.versionNodeAffinity(kernelFullVersion, obj, "spec", "nodeSelector"); err != nil {
			return errors.Wrap(err, "Cannot setup BuildConfig kernel version affinity")
		}
	}

	return nil
}

func (k *kernelData) versionNodeAffinity(kernelFullVersion string, obj *unstructured.Unstructured, fields ...string) error {

	nodeSelector, found, err := unstructured.NestedMap(obj.Object, fields...)
	if err != nil {
		return err
	}

	if !found {
		nodeSelector = make(map[string]interface{})
	}

	nodeSelector["feature.node.kubernetes.io/kernel-version.full"] = kernelFullVersion

	if err := unstructured.SetNestedMap(obj.Object, nodeSelector, fields...); err != nil {
		return errors.Wrap(err, "Cannot update nodeSelector")
	}

	return nil
}

func (k *kernelData) IsObjectAffine(obj client.Object) bool {

	annotations := obj.GetAnnotations()

	if affine, found := annotations["specialresource.openshift.io/kernel-affine"]; found && affine == "true" {
		return true
	}

	return false
}

func (k *kernelData) FullVersion(nodeList *corev1.NodeList) (string, error) {

	var found bool
	var kernelFullVersion string
	// Assuming all nodes are running the same kernel version,
	// one could easily add driver-kernel-versions for each node.
	for _, node := range nodeList.Items {
		labels := node.GetLabels()

		// We only need to check for the key, the value
		// is available if the key is there
		short := "feature.node.kubernetes.io/kernel-version.full"
		if kernelFullVersion, found = labels[short]; !found {
			return "", errors.New("Label " + short + " not found is NFD running? Check node labels")
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
