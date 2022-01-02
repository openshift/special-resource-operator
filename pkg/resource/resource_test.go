package resource_test

import (
	"context"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/kernel"
	"github.com/openshift-psap/special-resource-operator/pkg/lifecycle"
	"github.com/openshift-psap/special-resource-operator/pkg/metrics"
	"github.com/openshift-psap/special-resource-operator/pkg/poll"
	"github.com/openshift-psap/special-resource-operator/pkg/proxy"
	"github.com/openshift-psap/special-resource-operator/pkg/resource"
	buildv1 "github.com/openshift/api/build/v1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	kubetypes "k8s.io/apimachinery/pkg/types"
)

var (
	unstructuredMatcher = gomock.AssignableToTypeOf(&unstructured.Unstructured{})
)

const (
	ownedLabel = "specialresource.openshift.io/owned"
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

var _ = Describe("creator_CreateFromYAML", func() {
	var (
		ctrl          *gomock.Controller
		kubeClient    *clients.MockClientsInterface
		mockLifecycle *lifecycle.MockLifecycle
		metricsClient *metrics.MockMetrics
		pollActions   *poll.MockPollActions
		kernelData    *kernel.MockKernelData
		proxyAPI      *proxy.MockProxyAPI
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		kubeClient = clients.NewMockClientsInterface(ctrl)
		mockLifecycle = lifecycle.NewMockLifecycle(ctrl)
		metricsClient = metrics.NewMockMetrics(ctrl)
		pollActions = poll.NewMockPollActions(ctrl)
		kernelData = kernel.NewMockKernelData(ctrl)
		proxyAPI = proxy.NewMockProxyAPI(ctrl)
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	yamlSpec := []byte(`---
apiVersion: v1
kind: Pod
metadata:
  name: nginx
spec:
  containers:
  - name: nginx
    image: nginx:1.14.2
    ports:
    - containerPort: 80
  restartPolicy: Always
`)

	It("should not return an error when the resource is already there", func() {
		const (
			kernelFullVersion         = "1.2.3"
			namespace                 = "ns"
			operatingSystemMajorMinor = "8.5"
			specialResourceName       = "special-resource"
		)

		nodeSelector := map[string]string{"key": "value"}

		owner := v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "owner",
				Namespace: namespace,
			},
		}

		nsn := kubetypes.NamespacedName{
			Namespace: namespace,
			Name:      "nginx",
		}

		gomock.InOrder(
			kernelData.EXPECT().IsObjectAffine(gomock.Any()).Times(1).Return(false),
			kubeClient.EXPECT().Get(context.TODO(), nsn, unstructuredMatcher).Times(1),
			metricsClient.EXPECT().SetCompletedKind(specialResourceName, "Pod", "nginx", namespace, 1).Times(1),
		)

		scheme := runtime.NewScheme()

		err := v1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		err = resource.
			NewCreator(kubeClient, metricsClient, pollActions, kernelData, scheme, mockLifecycle, proxyAPI).
			CreateFromYAML(
				yamlSpec,
				false,
				&owner,
				specialResourceName,
				namespace,
				nodeSelector,
				kernelFullVersion,
				operatingSystemMajorMinor,
			)

		Expect(err).NotTo(HaveOccurred())
	})

	It("should create the resource when it is not already there", func() {
		const (
			kernelFullVersion         = "1.2.3"
			name                      = "nginx"
			namespace                 = "ns"
			operatingSystemMajorMinor = "8.5"
			ownerName                 = "owner"
			specialResourceName       = "special-resource"
		)

		nodeSelector := map[string]string{"key": "value"}

		owner := v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ownerName,
				Namespace: namespace,
			},
		}

		trueVar := true

		newPod := unstructured.Unstructured{}
		newPod.SetAPIVersion("v1")
		newPod.SetKind("Pod")
		newPod.SetName(name)
		newPod.SetNamespace(namespace)
		newPod.SetAnnotations(map[string]string{
			"meta.helm.sh/release-name":         specialResourceName,
			"meta.helm.sh/release-namespace":    namespace,
			"specialresource.openshift.io/hash": "5473155173593167161",
		})
		newPod.SetLabels(map[string]string{
			"app.kubernetes.io/managed-by": "Helm",
			ownedLabel:                     "true",
		})
		newPod.SetOwnerReferences([]metav1.OwnerReference{
			{
				APIVersion:         "v1",
				Kind:               "Pod",
				Name:               ownerName,
				BlockOwnerDeletion: &trueVar,
				Controller:         &trueVar,
			},
		})

		container := map[string]interface{}{
			"name":  "nginx",
			"image": "nginx:1.14.2",
			"ports": []interface{}{
				map[string]interface{}{"containerPort": int64(80)},
				// YAML deserializer converts all integers to int64, so use an int64 here as well
			},
		}

		// Setting this manually because unstructured.SetNestedMap struggles to deep copy the container ports
		newPod.Object["spec"] = map[string]interface{}{
			"containers": []interface{}{container},
		}

		err := unstructured.SetNestedStringMap(newPod.Object, nodeSelector, "spec", "nodeSelector")
		Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(newPod.Object, "Always", "spec", "restartPolicy")
		Expect(err).NotTo(HaveOccurred())

		nsn := kubetypes.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}

		gomock.InOrder(
			kernelData.EXPECT().IsObjectAffine(gomock.Any()).Times(1).Return(false),
			kubeClient.
				EXPECT().
				Get(context.TODO(), nsn, unstructuredMatcher).
				Return(errors.NewNotFound(v1.Resource("pod"), name)),
			kubeClient.
				EXPECT().
				Create(context.TODO(), &newPod),
			metricsClient.
				EXPECT().
				SetCompletedKind(specialResourceName, "Pod", name, namespace, 1),
		)

		scheme := runtime.NewScheme()

		err = v1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		err = resource.
			NewCreator(kubeClient, metricsClient, pollActions, kernelData, scheme, mockLifecycle, proxyAPI).
			CreateFromYAML(
				yamlSpec,
				false,
				&owner,
				specialResourceName,
				namespace,
				nodeSelector,
				kernelFullVersion,
				operatingSystemMajorMinor,
			)

		Expect(err).NotTo(HaveOccurred())
	})
})

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

var _ = Describe("SetLabel", func() {
	objs := []runtime.Object{
		&appsv1.DaemonSet{
			TypeMeta: metav1.TypeMeta{Kind: "DaemonSet"},
		},
		&appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{Kind: "Deployment"},
		},
		&appsv1.StatefulSet{
			TypeMeta: metav1.TypeMeta{Kind: "StatefulSet"},
		},
	}

	entries := make([]TableEntry, 0, len(objs))

	for _, o := range objs {
		entries = append(entries, Entry(o.GetObjectKind().GroupVersionKind().Kind, o))
	}

	testFunc := func(o client.Object) {
		mo, err := runtime.DefaultUnstructuredConverter.ToUnstructured(o)
		Expect(err).NotTo(HaveOccurred())

		uo := unstructured.Unstructured{Object: mo}

		// Create the map manually, otherwise SetLabel returns an error
		err = unstructured.SetNestedStringMap(uo.Object, map[string]string{}, "spec", "template", "metadata", "labels")
		Expect(err).NotTo(HaveOccurred())

		err = resource.SetLabel(&uo)
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

	DescribeTable("should the label", testFunc, entries...)

	It("should the label for BuildConfig", func() {
		bc := buildv1.BuildConfig{
			TypeMeta: metav1.TypeMeta{Kind: "BuildConfig"},
		}

		mo, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&bc)
		Expect(err).NotTo(HaveOccurred())

		uo := unstructured.Unstructured{Object: mo}

		err = resource.SetLabel(&uo)
		Expect(err).NotTo(HaveOccurred())
		Expect(uo.GetLabels()).To(HaveKeyWithValue(ownedLabel, "true"))
	})
})
