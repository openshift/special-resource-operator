package cluster_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	imagev1 "github.com/openshift/api/image/v1"
	"github.com/openshift/special-resource-operator/pkg/clients"
	"github.com/openshift/special-resource-operator/pkg/cluster"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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

	It("should return an error when client fails to access cluster version", func() {
		mockKubeClients.EXPECT().ClusterVersionGet(context.TODO(), metav1.GetOptions{}).Return(nil, fmt.Errorf("some error"))
		_, _, err := cluster.NewCluster(mockKubeClients).Version(context.TODO())
		Expect(err).To(HaveOccurred())
	})

	It("should return an error when the ClusterVersion does not have the expected history", func() {
		mockKubeClients.EXPECT().ClusterVersionGet(context.TODO(), metav1.GetOptions{}).Return(&configv1.ClusterVersion{}, nil)
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

			mockKubeClients.EXPECT().ClusterVersionGet(context.TODO(), metav1.GetOptions{}).Return(cv, nil)

			cvv, v, err := cluster.NewCluster(mockKubeClients).Version(context.TODO())
			Expect(err).NotTo(HaveOccurred())
			Expect(cvv).To(Equal(out0))
			Expect(v).To(Equal(out1))
		},
		Entry("version with a dot", "1.2", "1.2", "1.2"),
		Entry("version with no dot", "1", "1", "1"),
	)
})

var _ = Describe("cluster_OSImageURL", func() {
	const cmName = "machine-config-osimageurl"

	nsn := types.NamespacedName{
		Namespace: "openshift-machine-config-operator",
		Name:      cmName,
	}

	It("should return an error if the machine-config-osimageurl ConfigMap cannot be found", func() {
		cm := &unstructured.Unstructured{}
		cm.SetAPIVersion("v1")
		cm.SetKind("ConfigMap")

		errNotFound := k8serrors.NewNotFound(v1.Resource("configmaps"), cmName)

		mockKubeClients.EXPECT().Get(context.TODO(), nsn, cm).Return(errNotFound)
		_, err := cluster.NewCluster(mockKubeClients).OSImageURL(context.TODO())
		Expect(errors.Is(err, errNotFound)).To(BeTrue())
	})

	It("should return an error if the machine-config-osimageurl ConfigMap has no osImageURL field", func() {
		cm := &unstructured.Unstructured{}
		cm.SetAPIVersion("v1")
		cm.SetKind("ConfigMap")

		mockKubeClients.EXPECT().Get(context.TODO(), nsn, cm).
			Do(func(_ context.Context, _ types.NamespacedName, cm *unstructured.Unstructured) {
				err := unstructured.SetNestedStringMap(cm.Object, make(map[string]string), "data")
				Expect(err).NotTo(HaveOccurred())
			})

		_, err := cluster.NewCluster(mockKubeClients).OSImageURL(context.TODO())
		Expect(err).To(HaveOccurred())
	})

	It("should return the ConfigMap's osImageURL field if the machine-config-osimageurl ConfigMap can be found", func() {
		const osImageURLValue = "value"

		cm := &unstructured.Unstructured{}
		cm.SetAPIVersion("v1")
		cm.SetKind("ConfigMap")

		mockKubeClients.EXPECT().Get(context.TODO(), nsn, cm).
			Do(func(_ context.Context, _ types.NamespacedName, cm *unstructured.Unstructured) {
				err := unstructured.SetNestedStringMap(cm.Object, map[string]string{"osImageURL": osImageURLValue}, "data")
				Expect(err).NotTo(HaveOccurred())
			})

		s, err := cluster.NewCluster(mockKubeClients).OSImageURL(context.TODO())
		Expect(err).NotTo(HaveOccurred())
		Expect(s).To(Equal(s))
	})
})

var _ = Describe("cluster_OperatingSystem", func() {
	It("should work when nodeInfo includes correct OSImage", func() {
		nodesList := corev1.NodeList{
			Items: []corev1.Node{
				{
					Status: corev1.NodeStatus{
						NodeInfo: corev1.NodeSystemInfo{
							OSImage: "Red Hat Enterprise Linux CoreOS 49.84.202201102104-0 (Ootpa)",
						},
					},
				},
			},
		}
		osVersionMajor, osVersion, osVersionMajorMinor, err := cluster.NewCluster(nil).OperatingSystem(&nodesList)
		Expect(osVersionMajor).To(Equal("rhel8"))
		Expect(osVersion).To(Equal("rhel8.4"))
		Expect(osVersionMajorMinor).To(Equal("8.4"))
		Expect(err).NotTo(HaveOccurred())
	})

	It("should fail when node has invalid OSImage", func() {
		nodesList := corev1.NodeList{
			Items: []corev1.Node{
				{
					Status: corev1.NodeStatus{
						NodeInfo: corev1.NodeSystemInfo{
							OSImage: "Some OS with bad format",
						},
					},
				},
			},
		}
		_, _, _, err := cluster.NewCluster(nil).OperatingSystem(&nodesList)
		Expect(err).To(HaveOccurred())
	})

	It("should fail when node has no OSImage", func() {
		nodesList := corev1.NodeList{
			Items: make([]corev1.Node, 1),
		}
		_, _, _, err := cluster.NewCluster(nil).OperatingSystem(&nodesList)
		Expect(err).To(HaveOccurred())
	})

})

var _ = Describe("cluster_GetDTKImages", func() {
	It("should return an error in case of GET failure", func() {
		mockKubeClients.EXPECT().
			Get(gomock.Any(), types.NamespacedName{Namespace: "openshift", Name: "driver-toolkit"}, gomock.Any()).
			Return(randomError)

		_, err := cluster.NewCluster(mockKubeClients).GetDTKImages(context.TODO())
		Expect(err).To(HaveOccurred())
	})

	It("should return sorted slice of URLs", func() {
		const (
			remoteRegistryURL = "reg.io/release/repo"
			img1              = "sha256:1"
			img2              = "sha256:2"
			img3              = "sha256:3"
		)

		mockKubeClients.EXPECT().
			Get(gomock.Any(), types.NamespacedName{Namespace: "openshift", Name: "driver-toolkit"}, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ types.NamespacedName, is *imagev1.ImageStream) error {
				is.Status = imagev1.ImageStreamStatus{
					Tags: []imagev1.NamedTagEventList{
						{
							Tag: "latest",
							Items: []imagev1.TagEvent{
								{
									Created:              metav1.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
									DockerImageReference: remoteRegistryURL + "@" + img1,
								},
								{
									Created:              metav1.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
									DockerImageReference: remoteRegistryURL + "@" + img2,
								},
								{
									Created:              metav1.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC),
									DockerImageReference: remoteRegistryURL + "@" + img3,
								},
							},
						},
					},
				}
				return nil
			})

		urls, err := cluster.NewCluster(mockKubeClients).GetDTKImages(context.TODO())
		Expect(err).ToNot(HaveOccurred())
		// Sorted using Created field: newer first
		Expect(urls).To(Equal([]string{
			remoteRegistryURL + "@" + img3,
			remoteRegistryURL + "@" + img1,
			remoteRegistryURL + "@" + img2,
		}))
	})
})

var _ = Describe("cluster get next upgrade version", func() {
	It("should return an error when client fails to access cluster version", func() {
		mockKubeClients.EXPECT().ClusterVersionGet(context.TODO(), metav1.GetOptions{}).Return(nil, fmt.Errorf("some error"))
		_, err := cluster.NewCluster(mockKubeClients).NextUpgradeVersion(context.TODO())
		Expect(err).To(HaveOccurred())
	})

	It("should return an error when the ClusterVersion does not contain desired image", func() {
		mockKubeClients.EXPECT().ClusterVersionGet(context.TODO(), metav1.GetOptions{}).Return(&configv1.ClusterVersion{}, nil)
		_, err := cluster.NewCluster(mockKubeClients).NextUpgradeVersion(context.TODO())
		Expect(err).To(HaveOccurred())
	})

	It("should desired image", func() {
		cv := &configv1.ClusterVersion{
			Status: configv1.ClusterVersionStatus{
				Desired: configv1.Release{
					Image: "desiredImage",
				},
			},
		}
		mockKubeClients.EXPECT().ClusterVersionGet(context.TODO(), metav1.GetOptions{}).Return(cv, nil)
		image, err := cluster.NewCluster(mockKubeClients).NextUpgradeVersion(context.TODO())
		Expect(image).To(Equal("desiredImage"))
		Expect(err).NotTo(HaveOccurred())
	})
})

var _ = Describe("is cluster in upgrade", func() {
	It("should return an error when client fails to access cluster version", func() {
		mockKubeClients.EXPECT().ClusterVersionGet(context.TODO(), metav1.GetOptions{}).Return(nil, fmt.Errorf("some error"))
		_, err := cluster.NewCluster(mockKubeClients).IsClusterInUpgrade(context.TODO())
		Expect(err).To(HaveOccurred())
	})

	It("no completed versions", func() {
		mockKubeClients.EXPECT().ClusterVersionGet(context.TODO(), metav1.GetOptions{}).Return(&configv1.ClusterVersion{}, nil)
		inUpdate, err := cluster.NewCluster(mockKubeClients).IsClusterInUpgrade(context.TODO())
		Expect(err).NotTo(HaveOccurred())
		Expect(inUpdate).To(BeFalse())
	})

	It("desired image is equal to the latest completed", func() {
		firstTime := metav1.Now()
		secondTime := metav1.NewTime(firstTime.Time.Add(time.Minute * 120))
		thirdTime := metav1.NewTime(firstTime.Time.Add(time.Minute * 20))
		cv := &configv1.ClusterVersion{
			Status: configv1.ClusterVersionStatus{
				History: []configv1.UpdateHistory{
					{
						State:          "Completed",
						Image:          "compImage1",
						CompletionTime: &firstTime,
					},
					{
						State:          "Completed",
						Image:          "compImage2",
						CompletionTime: &secondTime,
					},
					{
						State:          "Completed",
						Image:          "compImage3",
						CompletionTime: &thirdTime,
					},
				},
				Desired: configv1.Release{
					Image: "compImage2",
				},
			},
		}
		mockKubeClients.EXPECT().ClusterVersionGet(context.TODO(), metav1.GetOptions{}).Return(cv, nil)
		inUpdate, err := cluster.NewCluster(mockKubeClients).IsClusterInUpgrade(context.TODO())
		Expect(err).NotTo(HaveOccurred())
		Expect(inUpdate).To(BeFalse())
	})

	It("desired image is not equal to the latest completed", func() {
		cv := &configv1.ClusterVersion{
			Status: configv1.ClusterVersionStatus{
				History: []configv1.UpdateHistory{
					{
						State: "Completed",
						Image: "completedImage",
					},
				},
				Desired: configv1.Release{
					Image: "desiredImage",
				},
			},
		}
		mockKubeClients.EXPECT().ClusterVersionGet(context.TODO(), metav1.GetOptions{}).Return(cv, nil)
		inUpdate, err := cluster.NewCluster(mockKubeClients).IsClusterInUpgrade(context.TODO())
		Expect(err).NotTo(HaveOccurred())
		Expect(inUpdate).To(BeTrue())
	})
})
