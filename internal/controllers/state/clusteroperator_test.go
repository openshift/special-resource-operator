package state_test

import (
	"context"
	"errors"
	"os"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	srov1beta1 "github.com/openshift-psap/special-resource-operator/api/v1beta1"
	"github.com/openshift-psap/special-resource-operator/internal/controllers/state"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	configv1 "github.com/openshift/api/config/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("ClusterOperatorManager", func() {
	const operatorName = "operator-name"

	var (
		mockKubeClient *clients.MockClientsInterface
		randomError    = errors.New("random error")
	)

	BeforeEach(func() {
		ctrl := gomock.NewController(GinkgoT())
		mockKubeClient = clients.NewMockClientsInterface(ctrl)
	})

	Describe("GetOrCreate", func() {
		It("should return an error if Kubernetes returned one", func() {
			mockKubeClient.
				EXPECT().
				ClusterOperatorGet(context.TODO(), operatorName, metav1.GetOptions{}).
				Return(nil, randomError)

			com := state.NewClusterOperatorManager(mockKubeClient, operatorName)

			err := com.GetOrCreate(context.TODO())
			Expect(err).To(HaveOccurred())
		})

		It("should not create a new ClusterOperator if one already exists", func() {
			co := &configv1.ClusterOperator{
				ObjectMeta: metav1.ObjectMeta{Name: operatorName},
			}

			mockKubeClient.
				EXPECT().
				ClusterOperatorGet(context.TODO(), operatorName, metav1.GetOptions{}).
				Return(co, nil)

			com := state.NewClusterOperatorManager(mockKubeClient, operatorName)

			err := com.GetOrCreate(context.TODO())
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a new ClusterOperator if none already exist", func() {
			newCO := configv1.ClusterOperator{
				ObjectMeta: metav1.ObjectMeta{Name: operatorName},
			}

			gomock.InOrder(
				mockKubeClient.
					EXPECT().
					ClusterOperatorGet(context.TODO(), operatorName, metav1.GetOptions{}).
					Return(nil, k8serrors.NewNotFound(configv1.Resource("clusteroperators"), operatorName)),
				mockKubeClient.
					EXPECT().
					ClusterOperatorCreate(context.TODO(), &newCO, metav1.CreateOptions{}).
					Return(&newCO, nil),
			)

			com := state.NewClusterOperatorManager(mockKubeClient, operatorName)

			err := com.GetOrCreate(context.TODO())
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Refresh", func() {
		It("should return an error if we cannot check if the ClusterOperator CRD is not available in this cluster", func() {
			mockKubeClient.
				EXPECT().
				HasResource(configv1.SchemeGroupVersion.WithResource("clusteroperators")).
				Return(false, randomError)

			com := state.NewClusterOperatorManager(mockKubeClient, "")

			err := com.Refresh(context.TODO(), nil)
			Expect(err).To(HaveOccurred())
		})

		It("should do nothing if the ClusterOperator CRD is not available in this cluster", func() {
			mockKubeClient.
				EXPECT().
				HasResource(configv1.SchemeGroupVersion.WithResource("clusteroperators")).
				Return(false, nil)

			com := state.NewClusterOperatorManager(mockKubeClient, "")

			err := com.Refresh(context.TODO(), nil)
			Expect(err).NotTo(HaveOccurred())

		})

		It("should work as expected", func() {
			co := configv1.ClusterOperator{
				ObjectMeta: metav1.ObjectMeta{Name: operatorName},
			}

			coWithRelatedObjects := co
			coWithRelatedObjects.Status.RelatedObjects = []configv1.ObjectReference{
				{Group: "", Resource: "namespaces", Name: os.Getenv("OPERATOR_NAMESPACE")},
				{Group: "sro.openshift.io", Resource: "specialresources", Name: ""},
			}

			gomock.InOrder(
				mockKubeClient.
					EXPECT().
					HasResource(configv1.SchemeGroupVersion.WithResource("clusteroperators")).
					Return(true, nil),
				mockKubeClient.
					EXPECT().
					ClusterOperatorGet(context.TODO(), operatorName, metav1.GetOptions{}).
					Return(&co, nil),
				mockKubeClient.EXPECT().List(context.TODO(), &srov1beta1.SpecialResourceList{}),
				mockKubeClient.EXPECT().ClusterOperatorUpdateStatus(context.TODO(), &coWithRelatedObjects, metav1.UpdateOptions{}),
			)
			com := state.NewClusterOperatorManager(mockKubeClient, operatorName)

			err := com.Refresh(context.TODO(), nil)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
