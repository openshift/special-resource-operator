package kernel

import (
	"io/ioutil"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	kernel kernelData
)

func TestKernel(t *testing.T) {
	RegisterFailHandler(Fail)

	BeforeEach(func() {
		kernel = kernelData{
			log: zap.New(zap.WriteTo(ioutil.Discard)),
		}
	})

	RunSpecs(t, "Kernel Suite")
}

const kernelFullVersion = "4.18.0-305.19.1.el8_4.x86_64"

func newObj(kind, name string) *unstructured.Unstructured {
	obj := unstructured.Unstructured{}
	obj.SetName(name)
	obj.SetKind(kind)

	return &obj
}

var _ = Describe("AffineAttributes", func() {
	const (
		objName                   = "test-obj"
		objNameHash               = "bfb16b50984f16f0" // fnv64a(operatingSystemMajorMinor + kernelFullVersion)
		objNewName                = objName + "-" + objNameHash
		operatingSystemMajorMinor = "8.4"
	)

	It("should work for BuildRun", func() {
		obj := newObj("BuildRun", objName)

		err := kernel.SetAffineAttributes(obj, kernelFullVersion, operatingSystemMajorMinor)

		Expect(err).NotTo(HaveOccurred())
		Expect(obj.GetName()).To(Equal(objNewName))
	})

	DescribeTable(
		"should work for these kinds",
		func(kind string) {
			obj := newObj(kind, objNewName)

			err := kernel.SetAffineAttributes(obj, kernelFullVersion, operatingSystemMajorMinor)
			Expect(err).NotTo(HaveOccurred())

			expectedSelector := map[string]interface{}{
				"feature.node.kubernetes.io/kernel-version.full": kernelFullVersion,
			}

			v, ok, err := unstructured.NestedMap(obj.Object, "spec", "nodeSelector")
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(v).To(Equal(expectedSelector))
		},
		Entry("Pod", "Pod"),
		Entry("BuildConfig", "BuildConfig"),
	)

	DescribeTable(
		"should work for more kinds",
		func(kind string) {
			obj := newObj(kind, objName)

			err := kernel.SetAffineAttributes(obj, kernelFullVersion, operatingSystemMajorMinor)
			Expect(err).NotTo(HaveOccurred())
			Expect(obj.GetLabels()).To(HaveKeyWithValue("app", objNewName))

			v, ok, err := unstructured.NestedString(obj.Object, "spec", "selector", "matchLabels", "app")
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(v).To(Equal(objNewName))

			v, ok, err = unstructured.NestedString(obj.Object, "spec", "template", "metadata", "labels", "app")
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(v).To(Equal(objNewName))

			// one if compares the kind to StatefulSet, the other one to StatefulSet (capital S)
			if kind != "StatefulSet" {
				expectedSelector := map[string]interface{}{
					"feature.node.kubernetes.io/kernel-version.full": kernelFullVersion,
				}

				var m map[string]interface{}

				m, ok, err = unstructured.NestedMap(obj.Object, "spec", "template", "spec", "nodeSelector")
				Expect(err).NotTo(HaveOccurred())
				Expect(ok).To(BeTrue())
				Expect(m).To(Equal(expectedSelector))
			}
		},
		Entry(nil, "DaemonSet"),
		Entry(nil, "Deployment"),
		Entry(nil, "StatefulSet"),
	)
})

var _ = Describe("SetVersionNodeAffinity", func() {
	DescribeTable(
		"should work for some kinds",
		func(kind string) {
			obj := newObj(kind, "")

			err := kernel.setVersionNodeAffinity(obj, kernelFullVersion)
			Expect(err).NotTo(HaveOccurred())

			expectedSelector := map[string]interface{}{
				"feature.node.kubernetes.io/kernel-version.full": kernelFullVersion,
			}

			v, ok, err := unstructured.NestedMap(obj.Object, "spec", "nodeSelector")
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(v).To(Equal(expectedSelector))
		},
		Entry("Pod", "Pod"),
		Entry("BuildConfig", "BuildConfig"),
	)

	DescribeTable(
		"should work for some kinds",
		func(kind string) {
			obj := newObj(kind, "")

			err := kernel.setVersionNodeAffinity(obj, kernelFullVersion)

			Expect(err).NotTo(HaveOccurred())

			expectedSelector := map[string]interface{}{
				"feature.node.kubernetes.io/kernel-version.full": kernelFullVersion,
			}

			m, ok, err := unstructured.NestedMap(obj.Object, "spec", "template", "spec", "nodeSelector")
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(m).To(Equal(expectedSelector))
		},
		Entry("DaemonSet", "DaemonSet"),
		Entry("Deployment", "Deployment"),
		Entry("Statefulset", "Statefulset"),
	)
})

var _ = Describe("TestIsObjectAffine", func() {
	It("should return false when not affine", func() {
		Expect(
			kernel.IsObjectAffine(&unstructured.Unstructured{}),
		).To(
			BeFalse(),
		)
	})

	It("should return true when affine", func() {
		obj := &unstructured.Unstructured{}
		obj.SetAnnotations(map[string]string{"specialresource.openshift.io/kernel-affine": "true"})

		Expect(kernel.IsObjectAffine(obj)).To(BeTrue())
	})
})

var _ = Describe("PatchVersion", func() {
	DescribeTable(
		"should return the expected value",
		func(input, expected string) {
			v, err := kernel.PatchVersion(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(Equal(expected))
		},
		EntryDescription("%q => %q"),
		Entry(nil, kernelFullVersion, "4.18.0-305"),
		Entry(nil, "4.18.0", "4.18.0"),
		Entry(nil, "4.18.0-305", "4.18.0-305"),
	)
})

var _ = Describe("FullVersion", func() {
	It("should return the version from the node", func() {
		const kernelVersion = "4.18.0-305.30.1.el8_4.x86_64"
		nodeList := corev1.NodeList{
			Items: []corev1.Node{
				{
					Status: corev1.NodeStatus{
						NodeInfo: corev1.NodeSystemInfo{
							KernelVersion: kernelVersion,
						},
					},
				},
			},
		}
		k, err := kernel.FullVersion(&nodeList)
		Expect(err).NotTo(HaveOccurred())
		Expect(k).To(Equal(kernelVersion))
	})
	It("should trigger an error if kernel not present", func() {
		nodeList := corev1.NodeList{
			Items: make([]corev1.Node, 1),
		}
		_, err := kernel.FullVersion(&nodeList)
		Expect(err).To(HaveOccurred())
	})
})
