package lifecycle_test

import (
	"context"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/lifecycle"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	name      = "test"
	namespace = "ns"
)

var (
	ctrl       *gomock.Controller
	labels     = map[string]string{"key": "value"}
	mockClient *clients.MockClientsInterface

	optNs     = client.InNamespace(namespace)
	optLabels = client.MatchingLabels(labels)

	unstructuredMatcher     = gomock.AssignableToTypeOf(&unstructured.Unstructured{})
	unstructuredListMatcher = gomock.AssignableToTypeOf(&unstructured.UnstructuredList{})
)

func TestLifecycle(t *testing.T) {
	RegisterFailHandler(Fail)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockClient = clients.NewMockClientsInterface(ctrl)

		clients.Interface = mockClient
	})

	AfterEach(func() {
		ctrl.Finish()
		clients.Interface = nil
	})

	RunSpecs(t, "Lifecycle Suite")
}

var _ = Describe("GetPodFromDaemonSet", func() {
	nsn := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}

	It("should be empty when DaemonSet does not exist", func() {
		err := errors.NewNotFound(v1.Resource("daemonset"), name)

		mockClient.EXPECT().
			Get(context.TODO(), nsn, gomock.Any()).
			Return(err)

		pl := lifecycle.GetPodFromDaemonSet(nsn)

		Expect(pl.Items).To(BeEmpty())
	})

	It("should return two pods when DaemonSet has 2 pods", func() {
		const nPod = 2

		gomock.InOrder(
			mockClient.EXPECT().
				Get(context.TODO(), nsn, unstructuredMatcher).
				Do(func(ctx context.Context, key types.NamespacedName, uo *unstructured.Unstructured) {
					m := make(map[string]interface{}, len(labels))

					for k, v := range labels {
						m[k] = v
					}

					err := unstructured.SetNestedMap(uo.Object, m, "spec", "selector", "matchLabels")
					Expect(err).NotTo(HaveOccurred())
					uo.SetNamespace(key.Namespace)
				}),
			mockClient.EXPECT().
				List(context.TODO(), unstructuredListMatcher, optNs, optLabels).
				Do(func(_ context.Context, ul *unstructured.UnstructuredList, _, _ client.ListOption) {
					ul.Items = make([]unstructured.Unstructured, nPod)
				}),
		)

		pl := lifecycle.GetPodFromDaemonSet(types.NamespacedName{Namespace: "ns", Name: "test"})

		Expect(pl.Items).To(HaveLen(nPod))
	})
})

var _ = Describe("UpdateDaemonSetPods", func() {
	const namespaceEnvVar = "OPERATOR_NAMESPACE"

	AfterEach(func() {
		err := os.Unsetenv(namespaceEnvVar)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should update the ConfigMap", func() {
		err := os.Setenv(namespaceEnvVar, namespace)
		Expect(err).NotTo(HaveOccurred())

		const cmName = "special-resource-lifecycle"

		nsn := types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}

		cmNsn := types.NamespacedName{
			Namespace: namespace,
			Name:      cmName,
		}

		// [TODO] - update the mocks, once storage package becomes typed interface with mock
		gomock.InOrder(
			mockClient.EXPECT().
				Get(context.TODO(), nsn, unstructuredMatcher).
				Do(func(_ context.Context, key types.NamespacedName, uo *unstructured.Unstructured) {
					m := make(map[string]interface{}, len(labels))

					for k, v := range labels {
						m[k] = v
					}

					err := unstructured.SetNestedMap(uo.Object, m, "spec", "selector", "matchLabels")
					Expect(err).NotTo(HaveOccurred())
					uo.SetNamespace(key.Namespace)
				}),
			mockClient.EXPECT().
				List(context.TODO(), unstructuredListMatcher, optNs, optLabels).
				Do(func(_ context.Context, ul *unstructured.UnstructuredList, _, _ client.ListOption) {
					pod1 := unstructured.Unstructured{}
					pod1.SetNamespace(namespace)
					pod1.SetName("pod1")

					pod2 := unstructured.Unstructured{}
					pod2.SetNamespace(namespace)
					pod2.SetName("pod2")

					ul.Items = []unstructured.Unstructured{pod1, pod2}
				}),
			mockClient.EXPECT().
				Get(context.TODO(), cmNsn, unstructuredMatcher),
			mockClient.EXPECT().
				Update(context.TODO(), unstructuredMatcher).
				Do(func(_ context.Context, uo *unstructured.Unstructured) {
					data, found, err := unstructured.NestedMap(uo.Object, "data")
					Expect(err).NotTo(HaveOccurred())
					Expect(found).To(BeTrue())
					Expect(data).To(HaveKeyWithValue("39005a809548c688", "*v1.Pod"))
				}),
			mockClient.EXPECT().
				Get(context.TODO(), cmNsn, unstructuredMatcher),
			mockClient.EXPECT().
				Update(context.TODO(), unstructuredMatcher).
				Do(func(_ context.Context, uo *unstructured.Unstructured) {
					data, found, err := unstructured.NestedMap(uo.Object, "data")
					Expect(err).NotTo(HaveOccurred())
					Expect(found).To(BeTrue())
					Expect(data).To(HaveKeyWithValue("39005d809548cba1", "*v1.Pod"))
				}),
		)

		obj := unstructured.Unstructured{}
		obj.SetNamespace(namespace)
		obj.SetName(name)

		err = lifecycle.UpdateDaemonSetPods(&obj)
		Expect(err).NotTo(HaveOccurred())
	})
})
