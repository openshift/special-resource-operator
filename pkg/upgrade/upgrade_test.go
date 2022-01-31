package upgrade

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/openshift-psap/special-resource-operator/pkg/cache"
	"github.com/openshift-psap/special-resource-operator/pkg/cluster"
	"github.com/openshift-psap/special-resource-operator/pkg/registry"
)

func TestPkgUpgrade(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Upgrade Suite")
}

// fakeLayer is a fake struct implementing github.com/google/go-containerregistry/pkg/v1.Layer interface
// The fake does not contain any logic because it's not directly accessed by the clusterInfo object,
// only handled using registry.Registry which is mocked.
type fakeLayer struct{}

func (fk *fakeLayer) Digest() (v1.Hash, error) {
	return v1.Hash{}, fmt.Errorf("not implemented")
}
func (fk *fakeLayer) DiffID() (v1.Hash, error) {
	return v1.Hash{}, fmt.Errorf("not implemented")
}
func (fk *fakeLayer) Compressed() (io.ReadCloser, error) {
	return nil, fmt.Errorf("not implemented")
}
func (fk *fakeLayer) Uncompressed() (io.ReadCloser, error) {
	return nil, fmt.Errorf("not implemented")
}
func (fk *fakeLayer) Size() (int64, error) {
	return 0, fmt.Errorf("not implemented")
}
func (fk *fakeLayer) MediaType() (types.MediaType, error) {
	return types.OCILayer, fmt.Errorf("not implemented")
}

var _ = Describe("ClusterInfo", func() {
	var (
		mockCtrl     *gomock.Controller
		mockRegistry *registry.MockRegistry
		mockCluster  *cluster.MockCluster
		clusterInfo  ClusterInfo
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockRegistry = registry.NewMockRegistry(mockCtrl)
		mockCluster = cluster.NewMockCluster(mockCtrl)
		clusterInfo = NewClusterInfo(mockRegistry, mockCluster)

		cache.Node = cache.NodesCache{
			List: &unstructured.UnstructuredList{
				Object: map[string]interface{}{},
				Items:  []unstructured.Unstructured{},
			},
			Count: 0,
		}
		cache.Node.List.SetAPIVersion("v1")
		cache.Node.List.SetKind("NodeList")
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	type testInput struct {
		nodesLabels          []map[string]string
		clusterVersion       string
		clusterReleaseImages []string
		dtkImage             string
		dtk                  registry.DriverToolkitEntry
	}

	kernel := "4.18.0-305.19.1.el8_4.x86_64"
	kernelRT := "4.18.0-305.19.1.rt7.91.el8_4.x86_64"
	system := "rhel"
	systemMajor := "8"
	systemMinor := "4"
	clusterVersion := "4.9"

	clusterReleaseImages := []string{"quay.io/release/release@sha256:1234567890abcdef"}
	dtkImageURL := "quay.io/dtk-image/dtk@sha256:1234567890abcdef"

	nodeLabelsWithRTKernel := map[string]string{
		labelKernelVersionFull:    kernelRT,
		labelOSReleaseVersionID:   clusterVersion,
		labelOSReleaseRHELVersion: fmt.Sprintf("%s.%s", systemMajor, systemMinor),
	}
	nodeLabelsWithRegularKernel := map[string]string{
		labelKernelVersionFull:    kernel,
		labelOSReleaseVersionID:   clusterVersion,
		labelOSReleaseRHELVersion: fmt.Sprintf("%s.%s", systemMajor, systemMinor),
	}
	clusterDTK := registry.DriverToolkitEntry{
		ImageURL:            "",
		KernelFullVersion:   kernel,
		RTKernelFullVersion: kernelRT,
		OSVersion:           fmt.Sprintf("%s.%s", systemMajor, systemMinor),
	}

	Context("has all required data (happy flow)", func() {
		testCases := []TableEntry{
			Entry(
				"1 node with RT kernel",
				testInput{
					nodesLabels:          []map[string]string{nodeLabelsWithRTKernel},
					clusterVersion:       clusterVersion,
					clusterReleaseImages: clusterReleaseImages,
					dtkImage:             dtkImageURL,
					dtk:                  clusterDTK,
				},
				map[string]NodeVersion{
					kernelRT: {
						OSVersion:      fmt.Sprintf("%s.%s", systemMajor, systemMinor),
						OSMajor:        fmt.Sprintf("%s%s", system, systemMajor),
						OSMajorMinor:   fmt.Sprintf("%s%s.%s", system, systemMajor, systemMinor),
						ClusterVersion: clusterVersion,
						DriverToolkit: registry.DriverToolkitEntry{
							ImageURL:            dtkImageURL,
							KernelFullVersion:   kernel,
							RTKernelFullVersion: kernelRT,
							OSVersion:           fmt.Sprintf("%s.%s", systemMajor, systemMinor),
						},
					},
				},
			),

			Entry(
				"1 node with non-RT kernel",
				testInput{
					nodesLabels:          []map[string]string{nodeLabelsWithRegularKernel},
					clusterVersion:       clusterVersion,
					clusterReleaseImages: clusterReleaseImages,
					dtkImage:             dtkImageURL,
					dtk:                  clusterDTK,
				},
				map[string]NodeVersion{
					kernel: {
						OSVersion:      fmt.Sprintf("%s.%s", systemMajor, systemMinor),
						OSMajor:        fmt.Sprintf("%s%s", system, systemMajor),
						OSMajorMinor:   fmt.Sprintf("%s%s.%s", system, systemMajor, systemMinor),
						ClusterVersion: clusterVersion,
						DriverToolkit: registry.DriverToolkitEntry{
							ImageURL:            dtkImageURL,
							KernelFullVersion:   kernel,
							RTKernelFullVersion: kernelRT,
							OSVersion:           fmt.Sprintf("%s.%s", systemMajor, systemMinor),
						},
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
					dtkImage:             dtkImageURL,
					dtk:                  clusterDTK,
				},
				map[string]NodeVersion{
					kernel: {
						OSVersion:      fmt.Sprintf("%s.%s", systemMajor, systemMinor),
						OSMajor:        fmt.Sprintf("%s%s", system, systemMajor),
						OSMajorMinor:   fmt.Sprintf("%s%s.%s", system, systemMajor, systemMinor),
						ClusterVersion: clusterVersion,
						DriverToolkit: registry.DriverToolkitEntry{
							ImageURL:            dtkImageURL,
							KernelFullVersion:   kernel,
							RTKernelFullVersion: kernelRT,
							OSVersion:           fmt.Sprintf("%s.%s", systemMajor, systemMinor),
						},
					},
					kernelRT: {
						OSVersion:      fmt.Sprintf("%s.%s", systemMajor, systemMinor),
						OSMajor:        fmt.Sprintf("%s%s", system, systemMajor),
						OSMajorMinor:   fmt.Sprintf("%s%s.%s", system, systemMajor, systemMinor),
						ClusterVersion: clusterVersion,
						DriverToolkit: registry.DriverToolkitEntry{
							ImageURL:            dtkImageURL,
							KernelFullVersion:   kernel,
							RTKernelFullVersion: kernelRT,
							OSVersion:           fmt.Sprintf("%s.%s", systemMajor, systemMinor),
						},
					},
				},
			),
		}

		DescribeTable("returns information for", func(input testInput, testExpects map[string]NodeVersion) {
			for _, labels := range input.nodesLabels {
				node := unstructured.Unstructured{}
				node.SetLabels(labels)
				cache.Node.List.Items = append(cache.Node.List.Items, node)
				cache.Node.Count = int64(len(cache.Node.List.Items))
			}

			ctx := context.TODO()

			mockCluster.EXPECT().VersionHistory(ctx).Return(input.clusterReleaseImages, nil)
			mockRegistry.EXPECT().LastLayer(ctx, input.clusterReleaseImages[0]).Return(&fakeLayer{}, nil)
			mockRegistry.EXPECT().ReleaseManifests(gomock.Any()).Return(input.clusterVersion, input.dtkImage, nil)
			mockRegistry.EXPECT().LastLayer(ctx, input.dtkImage).Return(&fakeLayer{}, nil)
			mockRegistry.EXPECT().ExtractToolkitRelease(gomock.Any()).Return(input.dtk, nil)

			m, err := clusterInfo.GetClusterInfo(ctx)

			Expect(err).ToNot(HaveOccurred())

			for expectedKernel, expectedNodeVersion := range testExpects {
				Expect(m).To(HaveKeyWithValue(expectedKernel, expectedNodeVersion))
			}
		}, testCases...)
	})

	It("will hint that with an error message when NFD is not installed", func() {
		cache.Node.List.Items = []unstructured.Unstructured{{}}
		cache.Node.Count = int64(len(cache.Node.List.Items))

		ctx := context.TODO()

		_, err := clusterInfo.GetClusterInfo(ctx)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("is NFD running?"))

		cache.Node.List.Items[0].SetLabels(map[string]string{
			labelKernelVersionFull: "fake",
		})
		_, err = clusterInfo.GetClusterInfo(ctx)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("is NFD running?"))
	})
})
