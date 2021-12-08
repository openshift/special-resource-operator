package resource_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/openshift-psap/special-resource-operator/pkg/resource"
	buildv1 "github.com/openshift/api/build/v1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestResource(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Resource Suite")
}

var _ = Describe("IsNamespaced", func() {
	Expect(resource.IsNamespaced("Pod")).To(BeTrue())

	clusterScopedTypes := []string{
		"Namespace",
		"ClusterRole",
		"ClusterRoleBinding",
		"SecurityContextConstraint",
		"SpecialResource",
	}

	entries := make([]TableEntry, 0, len(clusterScopedTypes))

	for _, cst := range clusterScopedTypes {
		entries = append(entries, Entry(cst, cst))
	}

	DescribeTable(
		"cluster-scoped types should not be namespaced",
		func(typeName string) {
			Expect(resource.IsNamespaced(typeName)).To(BeFalse())
		},
		entries...,
	)
})

var _ = Describe("IsNotUpdateable", func() {
	cases := []struct {
		typeName string
		matcher  types.GomegaMatcher
	}{
		{
			typeName: "Deployment",
			matcher:  BeFalse(),
		},
		{
			typeName: "ServiceAccount",
			matcher:  BeTrue(),
		},
		{
			typeName: "Pod",
			matcher:  BeTrue(),
		},
	}

	entries := make([]TableEntry, 0, len(cases))

	for _, c := range cases {
		entries = append(entries, Entry(c.typeName, c.typeName, c.matcher))
	}

	DescribeTable(
		"should not be updateable",
		func(typeName string, m types.GomegaMatcher) {
			Expect(resource.IsNotUpdateable(typeName)).To(m)
		},
		entries...,
	)
})

var _ = Describe("NeedsResourceVersionUpdate", func() {
	It("Pod should not requires a ResourceVersion", func() {
		Expect(resource.NeedsResourceVersionUpdate("Pod")).To(BeFalse())
	})

	resourceTypes := []string{
		"SecurityContextConstraints",
		"Service",
		"ServiceMonitor",
		"Route",
		"Build",
		"BuildRun",
		"BuildConfig",
		"ImageStream",
		"PrometheusRule",
		"CSIDriver",
		"Issuer",
		"CustomResourceDefinition",
		"Certificate",
		"SpecialResource",
		"OperatorGroup",
		"CertManager",
		"MutatingWebhookConfiguration",
		"ValidatingWebhookConfiguration",
		"Deployment",
		"ImagePolicy",
	}

	entries := make([]TableEntry, 0, len(resourceTypes))

	for _, rt := range resourceTypes {
		entries = append(entries, Entry(rt, rt))
	}

	DescribeTable(
		"requires a resource version update",
		func(rt string) {
			Expect(resource.NeedsResourceVersionUpdate(rt)).To(BeTrue())
		},
		entries...,
	)
})

var _ = Describe("UpdateResourceVersion", func() {
	It("should not return an error for Pod", func() {
		foundPod := v1.Pod{
			TypeMeta: metav1.TypeMeta{Kind: "Pod"},
		}

		foundMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&foundPod)
		Expect(err).NotTo(HaveOccurred())

		err = resource.UpdateResourceVersion(&unstructured.Unstructured{}, &unstructured.Unstructured{Object: foundMap})
		Expect(err).NotTo(HaveOccurred())
	})

	It("should return an error for Service with no resourceVersion", func() {
		foundSvc := v1.Service{
			TypeMeta: metav1.TypeMeta{Kind: "Service"},
		}

		foundMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&foundSvc)
		Expect(err).NotTo(HaveOccurred())

		err = resource.UpdateResourceVersion(&unstructured.Unstructured{}, &unstructured.Unstructured{Object: foundMap})
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

		err = resource.UpdateResourceVersion(&reqUnstructured, &unstructured.Unstructured{Object: foundMap})
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

		err = resource.UpdateResourceVersion(&reqUnstructured, &unstructured.Unstructured{Object: foundMap})
		Expect(err).NotTo(HaveOccurred())

		reqSvc := v1.Service{}

		err = runtime.DefaultUnstructuredConverter.FromUnstructured(reqUnstructured.Object, &reqSvc)
		Expect(err).NotTo(HaveOccurred())

		Expect(reqSvc.GetResourceVersion()).To(Equal(resourceVersion))
		Expect(reqSvc.Spec.ClusterIP).To(Equal(clusterIP))
	})
})

var _ = Describe("SetNodeSelectorTerms", func() {
	It("should work for a DaemonSet", func() {
		d := appsv1.DaemonSet{
			TypeMeta: metav1.TypeMeta{Kind: "DaemonSet"},
		}

		m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&d)
		Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedStringMap(m, make(map[string]string), "spec", "template", "spec", "nodeSelector")
		Expect(err).NotTo(HaveOccurred())

		terms := map[string]string{"key": "value"}
		uo := unstructured.Unstructured{Object: m}

		err = resource.SetNodeSelectorTerms(&uo, terms)
		Expect(err).NotTo(HaveOccurred())

		err = runtime.DefaultUnstructuredConverter.FromUnstructured(uo.Object, &d)
		Expect(err).NotTo(HaveOccurred())

		Expect(d.Spec.Template.Spec.NodeSelector).To(Equal(terms))
	})

	It("should work for a Deployment", func() {
		d := appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{Kind: "Deployment"},
		}

		m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&d)
		Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedStringMap(m, make(map[string]string), "spec", "template", "spec", "nodeSelector")
		Expect(err).NotTo(HaveOccurred())

		terms := map[string]string{"key": "value"}
		uo := unstructured.Unstructured{Object: m}

		err = resource.SetNodeSelectorTerms(&uo, terms)
		Expect(err).NotTo(HaveOccurred())

		err = runtime.DefaultUnstructuredConverter.FromUnstructured(uo.Object, &d)
		Expect(err).NotTo(HaveOccurred())

		Expect(d.Spec.Template.Spec.NodeSelector).To(Equal(terms))
	})

	// TODO(qbarrand) this bugs because the code checks if the kind is Statefulset (no capital S)
	PIt("should work for a StatefulSet", func() {
		statefulSet := appsv1.StatefulSet{
			TypeMeta: metav1.TypeMeta{Kind: "StatefulSet"},
		}

		m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&statefulSet)
		Expect(err).NotTo(HaveOccurred())

		terms := map[string]string{"key": "value"}
		uo := unstructured.Unstructured{Object: m}

		err = resource.SetNodeSelectorTerms(&uo, terms)
		Expect(err).NotTo(HaveOccurred())

		err = runtime.DefaultUnstructuredConverter.FromUnstructured(uo.Object, &statefulSet)
		Expect(err).NotTo(HaveOccurred())

		Expect(statefulSet.Spec.Template.Spec.NodeSelector).To(Equal(terms))
	})

	It("should work for a Pod", func() {
		p := v1.Pod{
			TypeMeta: metav1.TypeMeta{Kind: "Pod"},
		}

		m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&p)
		Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedStringMap(m, make(map[string]string), "spec", "nodeSelector")
		Expect(err).NotTo(HaveOccurred())

		terms := map[string]string{"key": "value"}
		uo := unstructured.Unstructured{Object: m}

		err = resource.SetNodeSelectorTerms(&uo, terms)
		Expect(err).NotTo(HaveOccurred())

		err = runtime.DefaultUnstructuredConverter.FromUnstructured(uo.Object, &p)
		Expect(err).NotTo(HaveOccurred())

		Expect(p.Spec.NodeSelector).To(Equal(terms))
	})

	It("should work for a BuildConfig", func() {
		d := buildv1.BuildConfig{
			TypeMeta: metav1.TypeMeta{Kind: "BuildConfig"},
		}

		m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&d)
		Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedStringMap(m, make(map[string]string), "spec", "nodeSelector")
		Expect(err).NotTo(HaveOccurred())

		terms := map[string]string{"key": "value"}
		uo := unstructured.Unstructured{Object: m}

		err = resource.SetNodeSelectorTerms(&uo, terms)
		Expect(err).NotTo(HaveOccurred())

		err = runtime.DefaultUnstructuredConverter.FromUnstructured(uo.Object, &d)
		Expect(err).NotTo(HaveOccurred())

		Expect(d.Spec.NodeSelector).To(Equal(buildv1.OptionalNodeSelector(terms)))
	})
})

// TODO(qbarrand)
//var _ = Describe("CreateFromYAML", func() {})

var _ = Describe("TestIsOneTimer", func() {
	It("should return false for Service", func() {
		svc := v1.Service{
			TypeMeta: metav1.TypeMeta{Kind: "Service"},
		}

		m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&svc)
		Expect(err).NotTo(HaveOccurred())

		ret, err := resource.IsOneTimer(&unstructured.Unstructured{Object: m})
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

			_, err = resource.IsOneTimer(&unstructured.Unstructured{Object: m})
			Expect(err).To(HaveOccurred())
		})

		It("should return true when restartPolicy=Never", func() {
			pod := v1.Pod{
				TypeMeta: metav1.TypeMeta{Kind: "Pod"},
				Spec:     v1.PodSpec{RestartPolicy: "Never"},
			}

			m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&pod)
			Expect(err).NotTo(HaveOccurred())

			ret, err := resource.IsOneTimer(&unstructured.Unstructured{Object: m})
			Expect(err).NotTo(HaveOccurred())

			Expect(ret).To(BeTrue())
		})
	})
})

// TODO(qbarrand)
//var _ = Describe("CRUD", func() {})

var _ = Describe("SetMetaData", func() {
	It("should set labels and annotations accordingly", func() {
		uo := unstructured.Unstructured{Object: make(map[string]interface{})}

		const (
			name      = "test-name"
			namespace = "test-namespace"
		)

		resource.SetMetaData(&uo, name, namespace)

		Expect(uo.GetAnnotations()).To(HaveKeyWithValue("meta.helm.sh/release-name", name))
		Expect(uo.GetAnnotations()).To(HaveKeyWithValue("meta.helm.sh/release-namespace", namespace))
		Expect(uo.GetLabels()).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "Helm"))
	})
})

// TODO(qbarrand)
//var _ = Describe("BeforeCRUD", func() {})

// TODO(qbarrand)
//var _ = Describe("AfterCRUD", func() {})
