package state_test

import (
	"context"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	"github.com/openshift/special-resource-operator/api/v1beta1"
	"github.com/openshift/special-resource-operator/internal/controllers/state"
	"github.com/openshift/special-resource-operator/pkg/clients"
	v1 "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("StatusUpdater", func() {
	var mockKubeClient *clients.MockClientsInterface

	BeforeEach(func() {
		ctrl := gomock.NewController(GinkgoT())
		mockKubeClient = clients.NewMockClientsInterface(ctrl)
	})

	Describe("UpdateWithState", func() {
		const (
			srName      = "sr-name"
			srNamespace = "sr-namespace"
		)

		It("should do nothing if the SpecialResource could not be found", func() {
			sr := &v1beta1.SpecialResource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      srName,
					Namespace: srNamespace,
				},
			}

			mockKubeClient.
				EXPECT().
				Get(context.TODO(), types.NamespacedName{Name: srName, Namespace: srNamespace}, &v1beta1.SpecialResource{}).
				Return(k8serrors.NewNotFound(v1.Resource("specialresources"), srName))

			state.NewStatusUpdater(mockKubeClient).UpdateWithState(context.TODO(), sr, "test")
		})

		It("should update the SpecialResource in Kubernetes if it exists", func() {
			const newState = "test"

			sr := &v1beta1.SpecialResource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      srName,
					Namespace: srNamespace,
				},
			}

			srWithState := sr.DeepCopy()
			srWithState.Status.State = newState

			gomock.InOrder(
				mockKubeClient.
					EXPECT().
					Get(context.TODO(), types.NamespacedName{Name: srName, Namespace: srNamespace}, &v1beta1.SpecialResource{}).
					Do(func(_ context.Context, _ types.NamespacedName, _ *v1beta1.SpecialResource) {
						sr.Status.State = newState
					}),
				mockKubeClient.EXPECT().StatusUpdate(context.TODO(), sr),
			)

			state.NewStatusUpdater(mockKubeClient).UpdateWithState(context.TODO(), sr, newState)
		})
	})
})
