package upgrade

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/openshift-psap/special-resource-operator/pkg/cluster"
	"github.com/openshift-psap/special-resource-operator/pkg/registry"
)

func TestPkgUpgrade(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Upgrade Suite")
}

var _ = Describe("ClusterInfo", func() {
	var (
		mockCtrl     *gomock.Controller
		mockRegistry *registry.MockRegistry
		mockCluster  *cluster.MockCluster
		clusterInfo  ClusterInfo
		nodesList    corev1.NodeList
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockRegistry = registry.NewMockRegistry(mockCtrl)
		mockCluster = cluster.NewMockCluster(mockCtrl)
		clusterInfo = NewClusterInfo(mockRegistry, mockCluster)
		nodesList = corev1.NodeList{}
		nodesList.Items = []corev1.Node{}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	type testInput struct {
		nodesLabels          []map[string]string
		clusterVersion       string
		clusterReleaseImages []string
	}

	kernel := "4.18.0-305.19.1.el8_4.x86_64"
	kernelRT := "4.18.0-305.19.1.rt7.91.el8_4.x86_64"
	system := "rhel"
	systemMajor := "8"
	systemMinor := "4"
	clusterVersion := "4.9"

	clusterReleaseImages := []string{"quay.io/release/release@sha256:1234567890abcdef"}

	nodeLabelsWithRTKernel := map[string]string{
		labelKernelVersionFull:       kernelRT,
		labelOSReleaseID:             system,
		labelOSReleaseVersionID:      clusterVersion,
		labelOSReleaseVersionIDMajor: systemMajor,
		labelOSReleaseVersionIDMinor: systemMinor,
	}
	nodeLabelsWithRegularKernel := map[string]string{
		labelKernelVersionFull:       kernel,
		labelOSReleaseID:             system,
		labelOSReleaseVersionID:      clusterVersion,
		labelOSReleaseVersionIDMajor: systemMajor,
		labelOSReleaseVersionIDMinor: systemMinor,
	}

	Context("has all required data (happy flow)", func() {
		DescribeTable("returns information for", func(input testInput, testExpects map[string]NodeVersion) {
			for _, labels := range input.nodesLabels {
				node := corev1.Node{}
				node.SetLabels(labels)
				nodesList.Items = append(nodesList.Items, node)
			}

			ctx := context.TODO()

			m, err := clusterInfo.GetClusterInfo(ctx, &nodesList)

			Expect(err).ToNot(HaveOccurred())

			for expectedKernel, expectedNodeVersion := range testExpects {
				Expect(m).To(HaveKeyWithValue(expectedKernel, expectedNodeVersion))
			}
		},
			Entry(
				"1 node with RT kernel",
				testInput{
					nodesLabels:          []map[string]string{nodeLabelsWithRTKernel},
					clusterVersion:       clusterVersion,
					clusterReleaseImages: clusterReleaseImages,
				},
				map[string]NodeVersion{
					kernelRT: {
						OSVersion:      fmt.Sprintf("%s.%s", systemMajor, systemMinor),
						OSMajor:        fmt.Sprintf("%s%s", system, systemMajor),
						OSMajorMinor:   fmt.Sprintf("%s%s.%s", system, systemMajor, systemMinor),
						ClusterVersion: clusterVersion,
					},
				},
			),

			Entry(
				"1 node with non-RT kernel",
				testInput{
					nodesLabels:          []map[string]string{nodeLabelsWithRegularKernel},
					clusterVersion:       clusterVersion,
					clusterReleaseImages: clusterReleaseImages,
				},
				map[string]NodeVersion{
					kernel: {
						OSVersion:      fmt.Sprintf("%s.%s", systemMajor, systemMinor),
						OSMajor:        fmt.Sprintf("%s%s", system, systemMajor),
						OSMajorMinor:   fmt.Sprintf("%s%s.%s", system, systemMajor, systemMinor),
						ClusterVersion: clusterVersion,
					},
				},
			),

			Entry(
				"2 nodes: 1st with RT, 2nd with non-RT kernel",
				testInput{
					nodesLabels: []map[string]string{
						nodeLabelsWithRTKernel,
						nodeLabelsWithRegularKernel,
					},
					clusterVersion:       clusterVersion,
					clusterReleaseImages: clusterReleaseImages,
				},
				map[string]NodeVersion{
					kernel: {
						OSVersion:      fmt.Sprintf("%s.%s", systemMajor, systemMinor),
						OSMajor:        fmt.Sprintf("%s%s", system, systemMajor),
						OSMajorMinor:   fmt.Sprintf("%s%s.%s", system, systemMajor, systemMinor),
						ClusterVersion: clusterVersion,
					},
					kernelRT: {
						OSVersion:      fmt.Sprintf("%s.%s", systemMajor, systemMinor),
						OSMajor:        fmt.Sprintf("%s%s", system, systemMajor),
						OSMajorMinor:   fmt.Sprintf("%s%s.%s", system, systemMajor, systemMinor),
						ClusterVersion: clusterVersion,
					},
				},
			),
		)
	})

	It("will hint that with an error message when NFD is not installed", func() {
		nodesList.Items = append(nodesList.Items, corev1.Node{})
		ctx := context.TODO()

		_, err := clusterInfo.GetClusterInfo(ctx, &nodesList)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("is NFD running?"))

		nodesList.Items[0].SetLabels(map[string]string{
			labelKernelVersionFull: "fake",
		})
		_, err = clusterInfo.GetClusterInfo(ctx, &nodesList)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("is NFD running?"))
	})
})
