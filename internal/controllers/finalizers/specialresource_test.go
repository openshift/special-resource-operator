package finalizers_test

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift-psap/special-resource-operator/api/v1beta1"
	"github.com/openshift-psap/special-resource-operator/internal/controllers/finalizers"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/poll"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var (
	mockKubeClient  *clients.MockClientsInterface
	mockPollActions *poll.MockPollActions
)

func TestFinalizers(t *testing.T) {
	RegisterFailHandler(Fail)

	BeforeEach(func() {
		ctrl := gomock.NewController(GinkgoT())
		mockKubeClient = clients.NewMockClientsInterface(ctrl)
		mockPollActions = poll.NewMockPollActions(ctrl)
	})

	RunSpecs(t, "Finalizers Suite")
}

var _ = Describe("specialResourceFinalizer_AddToSpecialResource", func() {
	It("should add the finalizer", func() {
		sr := &v1beta1.SpecialResource{}

		mockKubeClient.EXPECT().Update(context.TODO(), sr)

		err := finalizers.NewSpecialResourceFinalizer(mockKubeClient, nil).AddToSpecialResource(context.TODO(), sr)
		Expect(err).NotTo(HaveOccurred())
		Expect(controllerutil.ContainsFinalizer(sr, finalizers.FinalizerString)).To(BeTrue())
	})

	It("should return an error if the object could not be updated", func() {
		sr := &v1beta1.SpecialResource{}

		randomError := errors.New("random error")

		mockKubeClient.EXPECT().Update(context.TODO(), sr).Return(randomError)

		err := finalizers.NewSpecialResourceFinalizer(mockKubeClient, nil).AddToSpecialResource(context.TODO(), sr)
		Expect(err).To(Equal(randomError))
	})
})

var _ = Describe("specialResourceFinalizer_Finalize", func() {
	It("should do nothing if the CR does not have the finalizer", func() {
		sr := &v1beta1.SpecialResource{}

		err := finalizers.NewSpecialResourceFinalizer(mockKubeClient, nil).Finalize(context.TODO(), sr)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should perform the finalizing logic", func() {
		const (
			srName      = "sr-name"
			srNamespace = "sr-namespace"
		)

		nodeSelector := map[string]string{"key": "value"}

		sr := &v1beta1.SpecialResource{
			ObjectMeta: metav1.ObjectMeta{
				Name:       srName,
				Namespace:  srNamespace,
				Finalizers: []string{finalizers.FinalizerString},
			},
			Spec: v1beta1.SpecialResourceSpec{
				Namespace:    srNamespace,
				NodeSelector: nodeSelector,
			},
		}

		srWithoutFinalizer := sr.DeepCopy()
		srWithoutFinalizer.SetFinalizers([]string{})

		nodes := &v1.NodeList{
			Items: []v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"specialresource.openshift.io/state-sr-name": "some-value"},
					},
				},
			},
		}

		emptyNode := &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Labels: make(map[string]string),
			},
		}

		ns := unstructured.Unstructured{}
		ns.SetKind("Namespace")
		ns.SetAPIVersion("v1")
		ns.SetName(srNamespace)

		refs := []metav1.OwnerReference{
			{
				APIVersion: "v1beta1",
				Kind:       "SpecialResource",
			},
		}

		nsWithOwnerReference := ns.DeepCopy()
		nsWithOwnerReference.SetOwnerReferences(refs)

		gomock.InOrder(
			mockKubeClient.
				EXPECT().
				GetNodesByLabels(context.TODO(), nodeSelector).
				Return(nodes, nil),
			mockKubeClient.EXPECT().Update(context.TODO(), emptyNode),
			mockKubeClient.
				EXPECT().
				Get(context.TODO(), types.NamespacedName{Name: srNamespace}, &ns).
				Do(func(_ context.Context, _ types.NamespacedName, obj client.Object) {
					obj.SetOwnerReferences(refs)
				}),
			mockKubeClient.EXPECT().Delete(context.TODO(), nsWithOwnerReference),
			mockPollActions.EXPECT().ForResourceUnavailability(context.TODO(), nsWithOwnerReference),
			mockKubeClient.EXPECT().Update(context.TODO(), srWithoutFinalizer),
		)

		f := finalizers.NewSpecialResourceFinalizer(mockKubeClient, mockPollActions)

		err := f.Finalize(context.TODO(), sr)
		Expect(err).NotTo(HaveOccurred())
	})
})
