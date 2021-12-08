package proxy_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/openshift-psap/special-resource-operator/pkg/proxy"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestProxy(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Proxy Suite")
}

var _ = Describe("Setup", func() {
	AfterEach(func() {
		proxy.ProxyConfiguration = proxy.Configuration{}
	})

	It("should return an error for Pod with empty spec", func() {
		pod := v1.Pod{
			TypeMeta: metav1.TypeMeta{Kind: "Pod"},
		}

		m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&pod)
		Expect(err).NotTo(HaveOccurred())

		uo := unstructured.Unstructured{Object: m}

		err = proxy.Setup(&uo)
		Expect(err).To(HaveOccurred())
	})

	It("should return no error for Pod with one container", func() {
		const (
			httpProxy  = "http-host-with-proxy"
			httpsProxy = "https-host-with-proxy"
			noProxy    = "host-without-proxy"
		)

		proxy.ProxyConfiguration = proxy.Configuration{
			HttpProxy:  httpProxy,
			HttpsProxy: httpsProxy,
			NoProxy:    noProxy,
		}

		pod := v1.Pod{
			TypeMeta: metav1.TypeMeta{Kind: "Pod"},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name: "test",
						Env:  make([]v1.EnvVar, 0),
					},
				},
			},
		}

		m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&pod)
		Expect(err).NotTo(HaveOccurred())

		uo := unstructured.Unstructured{Object: m}

		err = proxy.Setup(&uo)
		Expect(err).NotTo(HaveOccurred())

		err = runtime.DefaultUnstructuredConverter.FromUnstructured(uo.Object, &pod)
		Expect(err).NotTo(HaveOccurred())

		// TODO(qbarrand) fix the method and then uncomment.
		// SetupPod does not set the resulting containers slice with unstructured.SetNestedSlice
		//env := pod.Spec.Containers[0].Env

		//assert.Contains(t, env, v1.EnvVar{Name: "HTTP_PROXY", Value: httpProxy})
		//assert.Contains(t, env, v1.EnvVar{Name: "HTTPS_PROXY", Value: httpsProxy})
		//assert.Contains(t, env, v1.EnvVar{Name: "NO_PROXY", Value: noProxy})
	})

	It("should return an error for DaemonSet with empty spec", func() {
		ds := appsv1.DaemonSet{
			TypeMeta: metav1.TypeMeta{Kind: "DaemonSet"},
		}

		m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&ds)
		Expect(err).NotTo(HaveOccurred())

		uo := unstructured.Unstructured{Object: m}

		err = proxy.Setup(&uo)
		Expect(err).To(HaveOccurred())
	})

	It("should return no error for DaemonSet with one container template", func() {
		const (
			httpProxy  = "http-host-with-proxy"
			httpsProxy = "https-host-with-proxy"
			noProxy    = "host-without-proxy"
		)

		proxy.ProxyConfiguration = proxy.Configuration{
			HttpProxy:  httpProxy,
			HttpsProxy: httpsProxy,
			NoProxy:    noProxy,
		}

		ds := appsv1.DaemonSet{
			TypeMeta: metav1.TypeMeta{Kind: "DaemonSet"},
			Spec: appsv1.DaemonSetSpec{
				Template: v1.PodTemplateSpec{
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{
								Name: "test",
								Env:  make([]v1.EnvVar, 0),
							},
						},
					},
				},
			},
		}

		m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&ds)
		Expect(err).NotTo(HaveOccurred())

		uo := unstructured.Unstructured{Object: m}

		err = proxy.Setup(&uo)
		Expect(err).NotTo(HaveOccurred())

		err = runtime.DefaultUnstructuredConverter.FromUnstructured(uo.Object, &ds)
		Expect(err).NotTo(HaveOccurred())

		// TODO(qbarrand) fix the method and then uncomment.
		// SetupDaemonSet does not set the resulting containers slice with unstructured.SetNestedSlice
		//env := ds.Spec.Template.Spec.Containers[0].Env

		//assert.Contains(t, env, v1.EnvVar{Name: "HTTP_PROXY", Value: httpProxy})
		//assert.Contains(t, env, v1.EnvVar{Name: "HTTPS_PROXY", Value: httpsProxy})
		//assert.Contains(t, env, v1.EnvVar{Name: "NO_PROXY", Value: noProxy})
	})
})

// TODO(qbarrand) make the DiscoveryClient in clients.HasResource injectable, so we can mock it.
//var _ = Describe("ClusterConfiguration", func() {})
