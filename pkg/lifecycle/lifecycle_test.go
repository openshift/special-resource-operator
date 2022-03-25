package lifecycle_test

import (
	"context"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift/special-resource-operator/pkg/clients"
	"github.com/openshift/special-resource-operator/pkg/lifecycle"
	"github.com/openshift/special-resource-operator/pkg/storage"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	name      = "test"
	namespace = "ns"
)

var (
	ctrl        *gomock.Controller
	labels      = map[string]string{"key": "value"}
	mockClient  *clients.MockClientsInterface
	mockStorage *storage.MockStorage

	optNs     = client.InNamespace(namespace)
	optLabels = client.MatchingLabels(labels)
)

func TestLifecycle(t *testing.T) {
	RegisterFailHandler(Fail)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockClient = clients.NewMockClientsInterface(ctrl)
		mockStorage = storage.NewMockStorage(ctrl)
	})

	AfterEach(func() {
		ctrl.Finish()
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
			Get(context.Background(), nsn, gomock.Any()).
			Return(err)

		pl := lifecycle.New(mockClient, mockStorage).GetPodFromDaemonSet(context.Background(), nsn)

		Expect(pl.Items).To(BeEmpty())
	})

	It("should return two pods when DaemonSet has 2 pods", func() {
		const nPod = 2

		gomock.InOrder(
			mockClient.EXPECT().
				Get(context.Background(), nsn, &appsv1.DaemonSet{}).
				Do(func(ctx context.Context, key types.NamespacedName, ds *appsv1.DaemonSet) {
					ds.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}
					ds.SetNamespace(key.Namespace)
				}),
			mockClient.EXPECT().
				List(context.Background(), &v1.PodList{}, optNs, optLabels).
				Do(func(_ context.Context, pl *v1.PodList, _, _ client.ListOption) {
					pl.Items = make([]v1.Pod, nPod)
				}),
		)

		pl := lifecycle.
			New(mockClient, mockStorage).
			GetPodFromDaemonSet(context.Background(), types.NamespacedName{Namespace: "ns", Name: "test"})

		Expect(pl.Items).To(HaveLen(nPod))
	})
})

var _ = Describe("GetPodFromDeployment", func() {
	nsn := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}

	It("should be empty when Deployment does not exist", func() {
		err := errors.NewNotFound(v1.Resource("deployment"), name)

		mockClient.EXPECT().
			Get(context.Background(), nsn, gomock.Any()).
			Return(err)

		pl := lifecycle.New(mockClient, mockStorage).GetPodFromDeployment(context.Background(), nsn)

		Expect(pl.Items).To(BeEmpty())
	})

	It("should return two pods when DaemonSet has 2 pods", func() {
		const nPod = 2

		gomock.InOrder(
			mockClient.EXPECT().
				Get(context.Background(), nsn, &appsv1.Deployment{}).
				Do(func(ctx context.Context, key types.NamespacedName, dp *appsv1.Deployment) {
					dp.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}
					dp.SetNamespace(key.Namespace)
				}),
			mockClient.EXPECT().
				List(context.Background(), &v1.PodList{}, optNs, optLabels).
				Do(func(_ context.Context, pl *v1.PodList, _, _ client.ListOption) {
					pl.Items = make([]v1.Pod, nPod)
				}),
		)

		pl := lifecycle.
			New(mockClient, mockStorage).
			GetPodFromDeployment(context.Background(), types.NamespacedName{Namespace: namespace, Name: name})

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

		nsn := types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}

		cmNsn := types.NamespacedName{
			Namespace: namespace,
			Name:      "special-resource-lifecycle",
		}

		gomock.InOrder(
			mockClient.EXPECT().
				Get(context.Background(), nsn, &appsv1.DaemonSet{}).
				Do(func(_ context.Context, key types.NamespacedName, ds *appsv1.DaemonSet) {
					ds.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}
					ds.SetNamespace(key.Namespace)
				}),
			mockClient.EXPECT().
				List(context.Background(), &v1.PodList{}, optNs, optLabels).
				Do(func(_ context.Context, pl *v1.PodList, _, _ client.ListOption) {
					pl.Items = []v1.Pod{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "pod1",
								Namespace: namespace,
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "pod2",
								Namespace: namespace,
							},
						},
					}
				}),
			mockStorage.EXPECT().UpdateConfigMapEntry(context.Background(), "39005a809548c688", "*v1.Pod", cmNsn),
			mockStorage.EXPECT().UpdateConfigMapEntry(context.Background(), "39005d809548cba1", "*v1.Pod", cmNsn),
		)

		obj := unstructured.Unstructured{}
		obj.SetNamespace(namespace)
		obj.SetName(name)

		err = lifecycle.New(mockClient, mockStorage).UpdateDaemonSetPods(context.Background(), &obj)
		Expect(err).NotTo(HaveOccurred())
	})
})
