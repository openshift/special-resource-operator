package preflight

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	v1stream "github.com/google/go-containerregistry/pkg/v1/stream"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/openshift/special-resource-operator/pkg/cluster"
	"github.com/openshift/special-resource-operator/pkg/helmer"
	"github.com/openshift/special-resource-operator/pkg/kernel"
	"github.com/openshift/special-resource-operator/pkg/metrics"
	"github.com/openshift/special-resource-operator/pkg/registry"
	"github.com/openshift/special-resource-operator/pkg/resource"
	"github.com/openshift/special-resource-operator/pkg/runtime"
	"github.com/openshift/special-resource-operator/pkg/upgrade"
)

const (
	firstDigestLayer              = "firstDigestLayer"
	upgradeKernelVersion          = "upgradeKenrelVersion"
	upgradePatchKernelVersion     = "upgradePatchKernelVersion"
	incorrectUpgradeKernelVersion = "incorrectUpgradeKenrelVersion"
	dsImage                       = "daemonSetImage"
	ocpImage                      = "ocpImage"
	layersRepo                    = "layersRepo"
	clusterVersion                = "clusterVersion"
	clusterMajorMinor             = "clusterMajorMinor"
	osImageURL                    = "osImageURL"
	dtkImage                      = "dtkImage"
)

var (
	ctrl               *gomock.Controller
	mockRegistryAPI    *registry.MockRegistry
	mockClusterAPI     *cluster.MockCluster
	mockClusterInfoAPI *upgrade.MockClusterInfo
	mockResourceAPI    *resource.MockResourceAPI
	mockHelmerAPI      *helmer.MockHelmer
	mockMetricsAPI     *metrics.MockMetrics
	mockRuntimeAPI     *runtime.MockRuntimeAPI
	mockKernelAPI      *kernel.MockKernelData
	p                  *preflight
)

func TestPreflight(t *testing.T) {
	RegisterFailHandler(Fail)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockRegistryAPI = registry.NewMockRegistry(ctrl)
		mockClusterAPI = cluster.NewMockCluster(ctrl)
		mockClusterInfoAPI = upgrade.NewMockClusterInfo(ctrl)
		mockResourceAPI = resource.NewMockResourceAPI(ctrl)
		mockHelmerAPI = helmer.NewMockHelmer(ctrl)
		mockMetricsAPI = metrics.NewMockMetrics(ctrl)
		mockRuntimeAPI = runtime.NewMockRuntimeAPI(ctrl)
		mockKernelAPI = kernel.NewMockKernelData(ctrl)
		p = NewPreflightAPI(mockRegistryAPI,
			mockClusterAPI,
			mockClusterInfoAPI,
			mockResourceAPI,
			mockHelmerAPI,
			mockMetricsAPI,
			mockRuntimeAPI,
			mockKernelAPI).(*preflight)
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	RunSpecs(t, "Preflight Suite")
}

var _ = Describe("handleYAMLsCheck", func() {

	It("get objects from yaml failure", func() {
		mockResourceAPI.EXPECT().GetObjectsFromYAML([]byte("some yaml")).Return(nil, fmt.Errorf("some error"))

		verified, err := p.handleYAMLsCheck(context.TODO(), "some yaml", upgradeKernelVersion)

		Expect(err).To(HaveOccurred())
		Expect(verified).To(BeFalse())
	})

	It("build config is present in the yamls list", func() {
		objList := prepareObjListForTest(3, true, true)

		mockResourceAPI.EXPECT().GetObjectsFromYAML([]byte("some yaml")).Return(objList, nil)

		verified, err := p.handleYAMLsCheck(context.TODO(), "some yaml", upgradeKernelVersion)

		Expect(err).NotTo(HaveOccurred())
		Expect(verified).To(BeTrue())
	})

	It("build config and daemonset are missing in the yamls list", func() {
		objList := prepareObjListForTest(3, false, false)

		mockResourceAPI.EXPECT().GetObjectsFromYAML([]byte("some yaml")).Return(objList, nil)

		verified, err := p.handleYAMLsCheck(context.TODO(), "some yaml", upgradeKernelVersion)

		Expect(err).NotTo(HaveOccurred())
		Expect(verified).To(BeTrue())
	})

	It("build config missing, daemonset present in the yamls list", func() {
		digestsList := []string{firstDigestLayer}
		digestLayer := v1stream.Layer{}
		dtk := &registry.DriverToolkitEntry{KernelFullVersion: upgradeKernelVersion}
		objList := prepareObjListForTest(3, false, true)

		mockResourceAPI.EXPECT().GetObjectsFromYAML([]byte("some yaml")).Return(objList, nil)
		mockRegistryAPI.EXPECT().GetLayersDigests(gomock.Any(), dsImage).Return(layersRepo, digestsList, nil, nil)
		mockRegistryAPI.EXPECT().GetLayerByDigest(layersRepo, firstDigestLayer, nil).Return(&digestLayer, nil)
		mockRegistryAPI.EXPECT().ExtractToolkitRelease(&digestLayer).Return(dtk, nil)

		verified, err := p.handleYAMLsCheck(context.TODO(), "some yaml", upgradeKernelVersion)

		Expect(err).NotTo(HaveOccurred())
		Expect(verified).To(BeTrue())
	})
})

var _ = Describe("daemonSetPreflightCheck", func() {
	It("valid image", func() {
		digestsList := []string{firstDigestLayer}
		digestLayer := v1stream.Layer{}
		dtk := &registry.DriverToolkitEntry{KernelFullVersion: upgradeKernelVersion}
		daemonObj := prepareDaemonSet()

		mockRegistryAPI.EXPECT().GetLayersDigests(gomock.Any(), dsImage).Return(layersRepo, digestsList, nil, nil)
		mockRegistryAPI.EXPECT().GetLayerByDigest(layersRepo, firstDigestLayer, nil).Return(&digestLayer, nil)
		mockRegistryAPI.EXPECT().ExtractToolkitRelease(&digestLayer).Return(dtk, nil)

		verified, err := p.daemonSetPreflightCheck(context.TODO(), daemonObj, upgradeKernelVersion)

		Expect(err).NotTo(HaveOccurred())
		Expect(verified).To(BeTrue())
	})

	It("image is not available", func() {
		daemonObj := prepareDaemonSet()

		mockRegistryAPI.EXPECT().GetLayersDigests(gomock.Any(), dsImage).Return(layersRepo, []string{}, nil, fmt.Errorf("some error"))

		verified, err := p.daemonSetPreflightCheck(context.TODO(), daemonObj, upgradeKernelVersion)

		Expect(err).NotTo(HaveOccurred())
		Expect(verified).To(BeFalse())
	})

	It("dtk kernel version is not correct", func() {
		digestsList := []string{firstDigestLayer}
		digestLayer := v1stream.Layer{}
		dtk := &registry.DriverToolkitEntry{KernelFullVersion: incorrectUpgradeKernelVersion}
		daemonObj := prepareDaemonSet()

		mockRegistryAPI.EXPECT().GetLayersDigests(gomock.Any(), dsImage).Return(layersRepo, digestsList, nil, nil)
		mockRegistryAPI.EXPECT().GetLayerByDigest(layersRepo, firstDigestLayer, nil).Return(&digestLayer, nil)
		mockRegistryAPI.EXPECT().ExtractToolkitRelease(&digestLayer).Return(dtk, nil)

		verified, err := p.daemonSetPreflightCheck(context.TODO(), daemonObj, upgradeKernelVersion)

		Expect(err).NotTo(HaveOccurred())
		Expect(verified).To(BeFalse())
	})

	It("dtk is missing", func() {
		digestsList := []string{firstDigestLayer}
		digestLayer := v1stream.Layer{}
		daemonObj := prepareDaemonSet()

		mockRegistryAPI.EXPECT().GetLayersDigests(gomock.Any(), dsImage).Return(layersRepo, digestsList, nil, nil)
		mockRegistryAPI.EXPECT().GetLayerByDigest(layersRepo, firstDigestLayer, nil).Return(&digestLayer, nil)
		mockRegistryAPI.EXPECT().ExtractToolkitRelease(&digestLayer).Return(nil, fmt.Errorf("some error"))

		verified, err := p.daemonSetPreflightCheck(context.TODO(), daemonObj, upgradeKernelVersion)

		Expect(err).NotTo(HaveOccurred())
		Expect(verified).To(BeFalse())
	})
})

var _ = Describe("PrepareRuntimeInfo", func() {
	It("good flow", func() {
		digestLayer := v1stream.Layer{}
		machineOsConf := "410.84.202203141348-0"
		dtk := &registry.DriverToolkitEntry{KernelFullVersion: upgradeKernelVersion}
		res := runtime.RuntimeInformation{}

		mockRuntimeAPI.EXPECT().InitRuntimeInfo().Return(&res)
		mockRegistryAPI.EXPECT().LastLayer(gomock.Any(), ocpImage).Return(&digestLayer, nil)
		mockRegistryAPI.EXPECT().ReleaseImageMachineOSConfig(&digestLayer).Return(machineOsConf, nil)
		mockRegistryAPI.EXPECT().LastLayer(gomock.Any(), ocpImage).Return(&digestLayer, nil)
		mockRegistryAPI.EXPECT().ReleaseManifests(&digestLayer).Return(dtkImage, nil)
		mockClusterInfoAPI.EXPECT().GetDTKData(gomock.Any(), dtkImage).Return(dtk, nil)
		mockKernelAPI.EXPECT().PatchVersion(upgradeKernelVersion).Return(upgradePatchKernelVersion, nil)
		mockClusterAPI.EXPECT().Version(gomock.Any()).Return(clusterVersion, clusterMajorMinor, nil)
		mockClusterAPI.EXPECT().OSImageURL(gomock.Any()).Return(osImageURL, nil)

		_, err := p.PrepareRuntimeInfo(context.TODO(), ocpImage)

		Expect(err).NotTo(HaveOccurred())
	})
})

func prepareObjListForTest(numItems int, buildConfigFlag, daemonSetFlag bool) *unstructured.UnstructuredList {
	objList := &unstructured.UnstructuredList{
		Items: []unstructured.Unstructured{},
	}

	for i := 0; i < numItems; i++ {
		item := unstructured.Unstructured{}
		item.SetKind(fmt.Sprintf("objKind%d", i))
		objList.Items = append(objList.Items, item)
	}
	if buildConfigFlag {
		buildItem := unstructured.Unstructured{}
		buildItem.SetKind("BuildConfig")
		objList.Items = append(objList.Items, buildItem)
	}
	if daemonSetFlag {
		objList.Items = append(objList.Items, *prepareDaemonSet())
	}
	return objList
}

func prepareDaemonSet() *unstructured.Unstructured {
	item := unstructured.Unstructured{}
	item.SetKind("DaemonSet")
	containersSlice := make([]interface{}, 1)
	imageSlice := map[string]interface{}{}
	containersSlice[0] = imageSlice
	err := unstructured.SetNestedField(imageSlice, dsImage, "image")
	Expect(err).NotTo(HaveOccurred())
	err = unstructured.SetNestedSlice(item.Object, containersSlice, "spec", "template", "spec", "containers")
	Expect(err).NotTo(HaveOccurred())
	return &item
}
