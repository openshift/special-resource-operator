package cluster_test

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/cluster"
	"github.com/openshift-psap/special-resource-operator/pkg/utils"
	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	v1 "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

var (
	ctrl            *gomock.Controller
	mockKubeClients *clients.MockClientsInterface
	randomError     = errors.New("random error")
)

func TestCluster(t *testing.T) {
	RegisterFailHandler(Fail)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockKubeClients = clients.NewMockClientsInterface(ctrl)
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	RunSpecs(t, "Cluster Suite")
}

var _ = Describe("cluster_Version", func() {
	It("should return an error when the cluster cannot get a ClusterVersion", func() {
		mockKubeClients.
			EXPECT().
			HasResource(configv1.SchemeGroupVersion.WithResource("clusterversions")).
			Return(false, randomError)

		_, _, err := cluster.NewCluster(mockKubeClients).Version(context.TODO())
		Expect(err).To(Equal(randomError))
	})

	It("should return empty values when the cluster has no ClusterVersion", func() {
		mockKubeClients.
			EXPECT().
			HasResource(configv1.SchemeGroupVersion.WithResource("clusterversions")).
			Return(false, nil)

		cvv, v, err := cluster.NewCluster(mockKubeClients).Version(context.TODO())
		Expect(err).NotTo(HaveOccurred())
		Expect(cvv).To(BeEmpty())
		Expect(v).To(BeEmpty())
	})

	It("should return an error when the ClusterVersion does not have the expected history", func() {
		gomock.InOrder(
			mockKubeClients.
				EXPECT().
				HasResource(configv1.SchemeGroupVersion.WithResource("clusterversions")).
				Return(true, nil),
			mockKubeClients.
				EXPECT().
				ClusterVersionGet(context.TODO(), metav1.GetOptions{}).
				Return(&configv1.ClusterVersion{}, nil),
		)

		_, _, err := cluster.NewCluster(mockKubeClients).Version(context.TODO())
		Expect(err).To(HaveOccurred())
	})

	DescribeTable(
		"should return expected values when the ClusterVersion has the expected condition",
		func(input, out0, out1 string) {
			cv := &configv1.ClusterVersion{
				Status: configv1.ClusterVersionStatus{
					History: []configv1.UpdateHistory{
						{
							State:   "Completed",
							Version: input,
						},
					},
				},
			}

			gomock.InOrder(
				mockKubeClients.
					EXPECT().
					HasResource(configv1.SchemeGroupVersion.WithResource("clusterversions")).
					Return(true, nil),
				mockKubeClients.
					EXPECT().
					ClusterVersionGet(context.TODO(), metav1.GetOptions{}).
					Return(cv, nil),
			)

			cvv, v, err := cluster.NewCluster(mockKubeClients).Version(context.TODO())
			Expect(err).NotTo(HaveOccurred())
			Expect(cvv).To(Equal(out0))
			Expect(v).To(Equal(out1))
		},
		Entry("version with a dot", "1.2", "1.2", "1.2"),
		Entry("version with no dot", "1", "1", "1"),
	)
})

var _ = Describe("cluster_VersionHistory", func() {
	It("should return an error when the cluster cannot get a ClusterVersion", func() {
		mockKubeClients.
			EXPECT().
			HasResource(configv1.SchemeGroupVersion.WithResource("clusterversions")).
			Return(true, randomError)

		_, err := cluster.NewCluster(mockKubeClients).VersionHistory(context.TODO())
		Expect(err).To(Equal(randomError))
	})

	It("should return an empty slice when the cluster has no ClusterVersion", func() {
		mockKubeClients.
			EXPECT().
			HasResource(configv1.SchemeGroupVersion.WithResource("clusterversions")).
			Return(false, nil)

		s, err := cluster.NewCluster(mockKubeClients).VersionHistory(context.TODO())
		Expect(err).NotTo(HaveOccurred())
		Expect(s).To(BeEmpty())
	})

	It("should return an error when ClusterVersionGet fails", func() {
		gomock.InOrder(
			mockKubeClients.
				EXPECT().
				HasResource(configv1.SchemeGroupVersion.WithResource("clusterversions")).
				Return(true, nil),
			mockKubeClients.
				EXPECT().
				ClusterVersionGet(context.TODO(), metav1.GetOptions{}).
				Return(nil, randomError),
		)

		_, err := cluster.NewCluster(mockKubeClients).VersionHistory(context.TODO())
		Expect(errors.Is(err, randomError)).To(BeTrue())
	})

	It("should all images when we can get a ClusterVersion", func() {
		cv := configv1.ClusterVersion{
			Status: configv1.ClusterVersionStatus{
				Desired: configv1.Release{
					Image: "desired-image",
				},
				History: []configv1.UpdateHistory{
					{
						State: configv1.CompletedUpdate,
						Image: "completed-0",
					},
					{
						State: configv1.PartialUpdate,
						Image: "partial-0",
					},
					{
						State: configv1.CompletedUpdate,
						Image: "completed-1",
					},
				},
			},
		}

		gomock.InOrder(
			mockKubeClients.
				EXPECT().
				HasResource(configv1.SchemeGroupVersion.WithResource("clusterversions")).
				Return(true, nil),
			mockKubeClients.
				EXPECT().
				ClusterVersionGet(context.TODO(), metav1.GetOptions{}).
				Return(&cv, nil),
		)

		s, err := cluster.NewCluster(mockKubeClients).VersionHistory(context.TODO())
		Expect(err).NotTo(HaveOccurred())
		Expect(s).To(Equal([]string{"desired-image", "completed-0", "completed-1"}))
	})
})

var _ = Describe("cluster_OSImageURL", func() {
	const cmName = "machine-config-osimageurl"

	nsn := types.NamespacedName{
		Namespace: "openshift-machine-config-operator",
		Name:      cmName,
	}

	It("should return an error when we cannot check if MachineConfig is available", func() {
		mockKubeClients.
			EXPECT().
			HasResource(machinev1.SchemeGroupVersion.WithResource("machineconfigs")).
			Return(true, randomError)

		_, err := cluster.NewCluster(mockKubeClients).OSImageURL(context.TODO())
		Expect(errors.Is(err, randomError)).To(BeTrue())
	})

	It("should return an empty string when MachineConfig is not available", func() {
		mockKubeClients.
			EXPECT().
			HasResource(machinev1.SchemeGroupVersion.WithResource("machineconfigs")).
			Return(false, nil)

		s, err := cluster.NewCluster(mockKubeClients).OSImageURL(context.TODO())
		Expect(err).NotTo(HaveOccurred())
		Expect(s).To(BeEmpty())
	})

	It("should return an error if the machine-config-osimageurl ConfigMap cannot be found", func() {
		cm := &unstructured.Unstructured{}
		cm.SetAPIVersion("v1")
		cm.SetKind("ConfigMap")

		errNotFound := k8serrors.NewNotFound(v1.Resource("configmaps"), cmName)

		gomock.InOrder(
			mockKubeClients.
				EXPECT().
				HasResource(machinev1.SchemeGroupVersion.WithResource("machineconfigs")).
				Return(true, nil),
			mockKubeClients.
				EXPECT().
				Get(context.TODO(), nsn, cm).
				Return(errNotFound),
		)

		_, err := cluster.NewCluster(mockKubeClients).OSImageURL(context.TODO())
		Expect(errors.Is(err, errNotFound)).To(BeTrue())
	})

	It("should return an error if the machine-config-osimageurl ConfigMap has no osImageURL field", func() {
		cm := &unstructured.Unstructured{}
		cm.SetAPIVersion("v1")
		cm.SetKind("ConfigMap")

		gomock.InOrder(
			mockKubeClients.
				EXPECT().
				HasResource(machinev1.SchemeGroupVersion.WithResource("machineconfigs")).
				Return(true, nil),
			mockKubeClients.
				EXPECT().
				Get(context.TODO(), nsn, cm).
				Do(func(_ context.Context, _ types.NamespacedName, cm *unstructured.Unstructured) {
					err := unstructured.SetNestedStringMap(cm.Object, make(map[string]string), "data")
					Expect(err).NotTo(HaveOccurred())
				}),
		)

		_, err := cluster.NewCluster(mockKubeClients).OSImageURL(context.TODO())
		Expect(err).To(HaveOccurred())
	})

	It("should return the ConfigMap's osImageURL field if the machine-config-osimageurl ConfigMap can be found", func() {
		const osImageURLValue = "value"

		cm := &unstructured.Unstructured{}
		cm.SetAPIVersion("v1")
		cm.SetKind("ConfigMap")

		gomock.InOrder(
			mockKubeClients.
				EXPECT().
				HasResource(machinev1.SchemeGroupVersion.WithResource("machineconfigs")).
				Return(true, nil),
			mockKubeClients.
				EXPECT().
				Get(context.TODO(), nsn, cm).
				Do(func(_ context.Context, _ types.NamespacedName, cm *unstructured.Unstructured) {
					err := unstructured.SetNestedStringMap(cm.Object, map[string]string{"osImageURL": osImageURLValue}, "data")
					Expect(err).NotTo(HaveOccurred())
				}),
		)

		s, err := cluster.NewCluster(mockKubeClients).OSImageURL(context.TODO())
		Expect(err).NotTo(HaveOccurred())
		Expect(s).To(Equal(s))
	})
})

var _ = Describe("cluster_OperatingSystem", func() {

	It("should return an error when feature.node.kubernetes.io/system-os_release.ID is empty", func() {
		nodesList := utils.CreateNodesList(1, nil)
		_, _, _, err := cluster.NewCluster(nil).OperatingSystem(nodesList)
		Expect(err).To(HaveOccurred())
	})

	It("should return an error when feature.node.kubernetes.io/system-os_release.VERSION_ID.major is empty", func() {
		labels := map[string]string{"feature.node.kubernetes.io/system-os_release.ID": "abcd"}
		nodesList := utils.CreateNodesList(1, labels)
		_, _, _, err := cluster.NewCluster(nil).OperatingSystem(nodesList)
		Expect(err).To(HaveOccurred())
	})

	It("should use RHEL version when it has 3 digits", func() {
		labels := map[string]string{
			"feature.node.kubernetes.io/system-os_release.ID":               "abc",
			"feature.node.kubernetes.io/system-os_release.VERSION_ID.major": "def",
			"feature.node.kubernetes.io/system-os_release.RHEL_VERSION":     "123",
		}
		nodesList := utils.CreateNodesList(1, labels)

		o0, o1, o2, err := cluster.NewCluster(nil).OperatingSystem(nodesList)
		Expect(err).NotTo(HaveOccurred())
		Expect(o0).To(Equal("rhel1"))
		Expect(o1).To(Equal("rhel123"))
		Expect(o2).To(Equal("1.3"))
	})

	It("should call osversion.RenderOperatingSystem when all labels are present", func() {
		labels := map[string]string{
			"feature.node.kubernetes.io/system-os_release.ID":               "123",
			"feature.node.kubernetes.io/system-os_release.VERSION_ID.major": "456",
			"feature.node.kubernetes.io/system-os_release.VERSION_ID.minor": "789",
		}
		nodesList := utils.CreateNodesList(1, labels)
		o0, o1, o2, err := cluster.NewCluster(nil).OperatingSystem(nodesList)
		Expect(err).NotTo(HaveOccurred())
		Expect(o0).To(Equal("123456"))
		Expect(o1).To(Equal("123456.789"))
		Expect(o2).To(Equal("456.789"))
	})
})
