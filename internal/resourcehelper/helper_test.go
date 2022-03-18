package resourcehelper_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/openshift/special-resource-operator/internal/resourcehelper"

	buildv1 "github.com/openshift/api/build/v1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestResource(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Resource Helpers Suite")
}

var _ = Describe("IsNamespaced", func() {
	rh := resourcehelper.New()

	It("should return true for Pod", func() {
		Expect(rh.IsNamespaced("Pod")).To(BeTrue())
	})

	DescribeTable(
		"cluster-scoped types should not be namespaced",
		func(typeName string) {
			Expect(rh.IsNamespaced(typeName)).To(BeFalse())
		},
		Entry(nil, "Namespace"),
		Entry(nil, "ClusterRole"),
		Entry(nil, "ClusterRoleBinding"),
		Entry(nil, "SecurityContextConstraint"),
		Entry(nil, "SpecialResource"),
	)
})

var _ = Describe("IsNotUpdateable", func() {
	rh := resourcehelper.New()

	DescribeTable(
		"should not be updateable",
		func(typeName string, m types.GomegaMatcher) {
			Expect(rh.IsNotUpdateable(typeName)).To(m)
		},
		EntryDescription("%s"),
		Entry(nil, "Deployment", BeFalse()),
		Entry(nil, "ServiceAccount", BeTrue()),
		Entry(nil, "Pod", BeTrue()),
	)
})

var _ = Describe("NeedsResourceVersionUpdate", func() {
	rh := resourcehelper.New()

	It("Pod should not requires a ResourceVersion", func() {
		Expect(rh.NeedsResourceVersionUpdate("Pod")).To(BeFalse())
	})

	DescribeTable(
		"requires a resource version update",
		func(rt string) {
			Expect(rh.NeedsResourceVersionUpdate(rt)).To(BeTrue())
		},
		Entry(nil, "SecurityContextConstraints"),
		Entry(nil, "Service"),
		Entry(nil, "ServiceMonitor"),
		Entry(nil, "Route"),
		Entry(nil, "Build"),
		Entry(nil, "BuildRun"),
		Entry(nil, "BuildConfig"),
		Entry(nil, "ImageStream"),
		Entry(nil, "PrometheusRule"),
		Entry(nil, "CSIDriver"),
		Entry(nil, "Issuer"),
		Entry(nil, "CustomResourceDefinition"),
		Entry(nil, "Certificate"),
		Entry(nil, "SpecialResource"),
		Entry(nil, "OperatorGroup"),
		Entry(nil, "CertManager"),
		Entry(nil, "MutatingWebhookConfiguration"),
		Entry(nil, "ValidatingWebhookConfiguration"),
		Entry(nil, "Deployment"),
		Entry(nil, "ImagePolicy"),
	)
})

var _ = Describe("UpdateResourceVersion", func() {
	rh := resourcehelper.New()

	It("should not return an error for Pod", func() {
		foundPod := v1.Pod{
			TypeMeta: metav1.TypeMeta{Kind: "Pod"},
		}

		foundMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&foundPod)
		Expect(err).NotTo(HaveOccurred())

		err = rh.UpdateResourceVersion(&unstructured.Unstructured{}, &unstructured.Unstructured{Object: foundMap})
		Expect(err).NotTo(HaveOccurred())
	})

	It("should return an error for Service with no resourceVersion", func() {
		foundSvc := v1.Service{
			TypeMeta: metav1.TypeMeta{Kind: "Service"},
		}

		foundMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&foundSvc)
		Expect(err).NotTo(HaveOccurred())

		err = rh.UpdateResourceVersion(&unstructured.Unstructured{}, &unstructured.Unstructured{Object: foundMap})
		Expect(err).To(HaveOccurred())
	})

	It("should return an error for Service with no clusterIP", func() {
		foundSvc := v1.Service{
			TypeMeta:   metav1.TypeMeta{Kind: "Service"},
			ObjectMeta: metav1.ObjectMeta{ResourceVersion: "123"},
		}

		foundMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&foundSvc)
		Expect(err).NotTo(HaveOccurred())

		reqUnstructured := unstructured.Unstructured{
			Object: make(map[string]interface{}),
		}

		err = rh.UpdateResourceVersion(&reqUnstructured, &unstructured.Unstructured{Object: foundMap})
		Expect(err).To(HaveOccurred())
	})

	It("should work as expected for Service with clusterIP", func() {
		const (
			clusterIP       = "1.2.3.4"
			resourceVersion = "123"
		)

		foundSvc := v1.Service{
			TypeMeta:   metav1.TypeMeta{Kind: "Service"},
			ObjectMeta: metav1.ObjectMeta{ResourceVersion: resourceVersion},
			Spec:       v1.ServiceSpec{ClusterIP: clusterIP},
		}

		foundMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&foundSvc)
		Expect(err).NotTo(HaveOccurred())

		reqUnstructured := unstructured.Unstructured{
			Object: make(map[string]interface{}),
		}

		err = rh.UpdateResourceVersion(&reqUnstructured, &unstructured.Unstructured{Object: foundMap})
		Expect(err).NotTo(HaveOccurred())

		reqSvc := v1.Service{}

		err = runtime.DefaultUnstructuredConverter.FromUnstructured(reqUnstructured.Object, &reqSvc)
		Expect(err).NotTo(HaveOccurred())

		Expect(reqSvc.GetResourceVersion()).To(Equal(resourceVersion))
		Expect(reqSvc.Spec.ClusterIP).To(Equal(clusterIP))
	})
})

var _ = Describe("SetNodeSelectorTerms", func() {
	rh := resourcehelper.New()

	It("should work for a DaemonSet", func() {
		d := appsv1.DaemonSet{
			TypeMeta: metav1.TypeMeta{Kind: "DaemonSet"},
		}

		m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&d)
		Expect(err).NotTo(HaveOccurred())

		terms := map[string]string{"key": "value"}
		uo := unstructured.Unstructured{Object: m}

		err = rh.SetNodeSelectorTerms(&uo, terms)
		Expect(err).NotTo(HaveOccurred())

		err = runtime.DefaultUnstructuredConverter.FromUnstructured(uo.Object, &d)
		Expect(err).NotTo(HaveOccurred())

		expectedTerms := []v1.NodeSelectorTerm{
			v1.NodeSelectorTerm{
				MatchExpressions: []v1.NodeSelectorRequirement{
					v1.NodeSelectorRequirement{
						Key:      "key",
						Operator: v1.NodeSelectorOpIn,
						Values:   []string{"value"},
					},
				},
			},
		}

		Expect(d.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms).To(Equal(expectedTerms))
	})

	It("should work for a Deployment", func() {
		d := appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{Kind: "Deployment"},
		}

		m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&d)
		Expect(err).NotTo(HaveOccurred())

		terms := map[string]string{"key": "value"}
		uo := unstructured.Unstructured{Object: m}

		err = rh.SetNodeSelectorTerms(&uo, terms)
		Expect(err).NotTo(HaveOccurred())

		err = runtime.DefaultUnstructuredConverter.FromUnstructured(uo.Object, &d)
		Expect(err).NotTo(HaveOccurred())

		expectedTerms := []v1.NodeSelectorTerm{
			v1.NodeSelectorTerm{
				MatchExpressions: []v1.NodeSelectorRequirement{
					v1.NodeSelectorRequirement{
						Key:      "key",
						Operator: v1.NodeSelectorOpIn,
						Values:   []string{"value"},
					},
				},
			},
		}

		Expect(d.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms).To(Equal(expectedTerms))
	})

	It("should work for a StatefulSet", func() {
		statefulSet := appsv1.StatefulSet{
			TypeMeta: metav1.TypeMeta{Kind: "StatefulSet"},
		}

		m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&statefulSet)
		Expect(err).NotTo(HaveOccurred())

		terms := map[string]string{"key": "value"}
		uo := unstructured.Unstructured{Object: m}

		err = rh.SetNodeSelectorTerms(&uo, terms)
		Expect(err).NotTo(HaveOccurred())

		err = runtime.DefaultUnstructuredConverter.FromUnstructured(uo.Object, &statefulSet)
		Expect(err).NotTo(HaveOccurred())

		expectedTerms := []v1.NodeSelectorTerm{
			v1.NodeSelectorTerm{
				MatchExpressions: []v1.NodeSelectorRequirement{
					v1.NodeSelectorRequirement{
						Key:      "key",
						Operator: v1.NodeSelectorOpIn,
						Values:   []string{"value"},
					},
				},
			},
		}

		Expect(statefulSet.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms).To(Equal(expectedTerms))
	})

	It("should work for a Pod", func() {
		p := v1.Pod{
			TypeMeta: metav1.TypeMeta{Kind: "Pod"},
		}

		m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&p)
		Expect(err).NotTo(HaveOccurred())

		terms := map[string]string{"key": "value"}
		uo := unstructured.Unstructured{Object: m}

		err = rh.SetNodeSelectorTerms(&uo, terms)
		Expect(err).NotTo(HaveOccurred())

		err = runtime.DefaultUnstructuredConverter.FromUnstructured(uo.Object, &p)
		Expect(err).NotTo(HaveOccurred())

		expectedTerms := []v1.NodeSelectorTerm{
			v1.NodeSelectorTerm{
				MatchExpressions: []v1.NodeSelectorRequirement{
					v1.NodeSelectorRequirement{
						Key:      "key",
						Operator: v1.NodeSelectorOpIn,
						Values:   []string{"value"},
					},
				},
			},
		}

		Expect(p.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms).To(Equal(expectedTerms))
	})

})

var _ = Describe("TestIsOneTimer", func() {
	rh := resourcehelper.New()

	It("should return false for Service", func() {
		svc := v1.Service{
			TypeMeta: metav1.TypeMeta{Kind: "Service"},
		}

		m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&svc)
		Expect(err).NotTo(HaveOccurred())

		ret, err := rh.IsOneTimer(&unstructured.Unstructured{Object: m})
		Expect(err).NotTo(HaveOccurred())
		Expect(ret).To(BeFalse())
	})

	When("Pod", func() {
		It("should return an error when restartPolicy undefined", func() {
			pod := v1.Pod{
				TypeMeta: metav1.TypeMeta{Kind: "Pod"},
			}

			m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&pod)
			Expect(err).NotTo(HaveOccurred())

			_, err = rh.IsOneTimer(&unstructured.Unstructured{Object: m})
			Expect(err).To(HaveOccurred())
		})

		It("should return true when restartPolicy=Never", func() {
			pod := v1.Pod{
				TypeMeta: metav1.TypeMeta{Kind: "Pod"},
				Spec:     v1.PodSpec{RestartPolicy: "Never"},
			}

			m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&pod)
			Expect(err).NotTo(HaveOccurred())

			ret, err := rh.IsOneTimer(&unstructured.Unstructured{Object: m})
			Expect(err).NotTo(HaveOccurred())

			Expect(ret).To(BeTrue())
		})
	})
})

var _ = Describe("SetMetaData", func() {
	rh := resourcehelper.New()

	It("should set labels and annotations accordingly", func() {
		uo := unstructured.Unstructured{Object: make(map[string]interface{})}

		const (
			name      = "test-name"
			namespace = "test-namespace"
		)

		rh.SetMetaData(&uo, name, namespace)

		Expect(uo.GetAnnotations()).To(HaveKeyWithValue("meta.helm.sh/release-name", name))
		Expect(uo.GetAnnotations()).To(HaveKeyWithValue("meta.helm.sh/release-namespace", namespace))
		Expect(uo.GetLabels()).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "Helm"))
	})
})

var _ = Describe("SetLabel", func() {
	rh := resourcehelper.New()
	ownedLabel := "specialresource.openshift.io/owned"

	testFunc := func(o client.Object) {
		mo, err := runtime.DefaultUnstructuredConverter.ToUnstructured(o)
		Expect(err).NotTo(HaveOccurred())

		uo := unstructured.Unstructured{Object: mo}

		// Create the map manually, otherwise SetLabel returns an error
		err = unstructured.SetNestedStringMap(uo.Object, map[string]string{}, "spec", "template", "metadata", "labels")
		Expect(err).NotTo(HaveOccurred())

		err = rh.SetLabel(&uo, ownedLabel)
		Expect(err).NotTo(HaveOccurred())

		Expect(uo.GetLabels()).To(HaveKeyWithValue(ownedLabel, "true"))

		v, found, err := unstructured.NestedString(
			uo.Object,
			"spec",
			"template",
			"metadata",
			"labels",
			ownedLabel)

		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(v).To(Equal("true"))
	}

	DescribeTable(
		"should the label",
		testFunc,
		func(o client.Object) string { return o.GetObjectKind().GroupVersionKind().Kind },
		Entry(
			nil,
			&appsv1.DaemonSet{
				TypeMeta: metav1.TypeMeta{Kind: "DaemonSet"},
			},
		),
		Entry(
			nil,
			&appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{Kind: "Deployment"},
			},
		),
		Entry(
			nil,
			&appsv1.StatefulSet{
				TypeMeta: metav1.TypeMeta{Kind: "StatefulSet"},
			},
		),
	)

	It("should the label for BuildConfig", func() {
		bc := buildv1.BuildConfig{
			TypeMeta: metav1.TypeMeta{Kind: "BuildConfig"},
		}

		mo, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&bc)
		Expect(err).NotTo(HaveOccurred())

		uo := unstructured.Unstructured{Object: mo}

		err = rh.SetLabel(&uo, ownedLabel)
		Expect(err).NotTo(HaveOccurred())
		Expect(uo.GetLabels()).To(HaveKeyWithValue(ownedLabel, "true"))
	})
})
