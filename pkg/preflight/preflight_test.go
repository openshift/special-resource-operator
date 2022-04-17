package preflight

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	v1stream "github.com/google/go-containerregistry/pkg/v1/stream"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8sv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"

	"github.com/openshift/special-resource-operator/pkg/cluster"
	"github.com/openshift/special-resource-operator/pkg/helmer"
	"github.com/openshift/special-resource-operator/pkg/kernel"
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
	dsName                        = "daemonSetName"
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
		mockRuntimeAPI = runtime.NewMockRuntimeAPI(ctrl)
		mockKernelAPI = kernel.NewMockKernelData(ctrl)
		p = NewPreflightAPI(mockRegistryAPI,
			mockClusterAPI,
			mockClusterInfoAPI,
			mockResourceAPI,
			mockHelmerAPI,
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

		verified, message, err := p.handleYAMLsCheck(context.TODO(), "some yaml", upgradeKernelVersion)

		Expect(err).To(HaveOccurred())
		Expect(verified).To(BeFalse())
		Expect(message).To(Equal("Failed to extract object from chart yaml list during preflight"))
	})

	It("number of build configs in the yamls list is equal to number of daemon sets", func() {
		objList := prepareObjListForTest(3, 2, 2)

		mockResourceAPI.EXPECT().GetObjectsFromYAML([]byte("some yaml")).Return(objList, nil)

		verified, message, err := p.handleYAMLsCheck(context.TODO(), "some yaml", upgradeKernelVersion)

		Expect(err).NotTo(HaveOccurred())
		Expect(verified).To(BeTrue())
		Expect(message).To(Equal(VerificationStatusReasonBuildConfigPresent))
	})

	It("build config and daemonset are missing in the yamls list", func() {
		objList := prepareObjListForTest(3, 0, 0)

		mockResourceAPI.EXPECT().GetObjectsFromYAML([]byte("some yaml")).Return(objList, nil)

		verified, message, err := p.handleYAMLsCheck(context.TODO(), "some yaml", upgradeKernelVersion)

		Expect(err).NotTo(HaveOccurred())
		Expect(verified).To(BeTrue())
		Expect(message).To(Equal(VerificationStatusReasonNoDaemonSet))
	})

	It("number of build configs in the yamls list is less to number of daemon sets", func() {
		digestsList := []string{firstDigestLayer}
		digestLayer := v1stream.Layer{}
		dtk := &registry.DriverToolkitEntry{KernelFullVersion: upgradeKernelVersion}
		objList := prepareObjListForTest(3, 2, 4)

		mockResourceAPI.EXPECT().GetObjectsFromYAML([]byte("some yaml")).Return(objList, nil)
		mockRegistryAPI.EXPECT().GetLayersDigests(gomock.Any(), dsImage).Return(layersRepo, digestsList, nil, nil).Times(2)
		mockRegistryAPI.EXPECT().GetLayerByDigest(layersRepo, firstDigestLayer, nil).Return(&digestLayer, nil).Times(2)
		mockRegistryAPI.EXPECT().ExtractToolkitRelease(&digestLayer).Return(dtk, nil).Times(2)

		verified, message, err := p.handleYAMLsCheck(context.TODO(), "some yaml", upgradeKernelVersion)

		Expect(err).NotTo(HaveOccurred())
		Expect(verified).To(BeTrue())
		Expect(message).To(Equal(VerificationStatusReasonVerified))
	})
})

var _ = Describe("daemonSetPreflightCheck", func() {
	It("valid image", func() {
		digestsList := []string{firstDigestLayer}
		digestLayer := v1stream.Layer{}
		dtk := &registry.DriverToolkitEntry{KernelFullVersion: upgradeKernelVersion}
		daemonObj := prepareDaemonSet("driver-module")

		mockRegistryAPI.EXPECT().GetLayersDigests(gomock.Any(), dsImage).Return(layersRepo, digestsList, nil, nil)
		mockRegistryAPI.EXPECT().GetLayerByDigest(layersRepo, firstDigestLayer, nil).Return(&digestLayer, nil)
		mockRegistryAPI.EXPECT().ExtractToolkitRelease(&digestLayer).Return(dtk, nil)

		verified, message, err := p.daemonSetPreflightCheck(context.TODO(), daemonObj, upgradeKernelVersion)

		Expect(err).NotTo(HaveOccurred())
		Expect(verified).To(BeTrue())
		Expect(message).To(Equal(VerificationStatusReasonVerified))
	})

	It("image is not available", func() {
		daemonObj := prepareDaemonSet("driver-module")

		mockRegistryAPI.EXPECT().GetLayersDigests(gomock.Any(), dsImage).Return(layersRepo, []string{}, nil, fmt.Errorf("some error"))

		verified, message, err := p.daemonSetPreflightCheck(context.TODO(), daemonObj, upgradeKernelVersion)

		Expect(err).NotTo(HaveOccurred())
		Expect(verified).To(BeFalse())
		Expect(message).To(Equal(fmt.Sprintf("DaemonSet %s, image %s inaccessible or does not exists", dsName, dsImage)))
	})

	It("dtk kernel version is not correct", func() {
		digestsList := []string{firstDigestLayer}
		digestLayer := v1stream.Layer{}
		dtk := &registry.DriverToolkitEntry{KernelFullVersion: incorrectUpgradeKernelVersion}
		daemonObj := prepareDaemonSet("driver-module")

		mockRegistryAPI.EXPECT().GetLayersDigests(gomock.Any(), dsImage).Return(layersRepo, digestsList, nil, nil)
		mockRegistryAPI.EXPECT().GetLayerByDigest(layersRepo, firstDigestLayer, nil).Return(&digestLayer, nil)
		mockRegistryAPI.EXPECT().ExtractToolkitRelease(&digestLayer).Return(dtk, nil)

		verified, message, err := p.daemonSetPreflightCheck(context.TODO(), daemonObj, upgradeKernelVersion)

		Expect(err).NotTo(HaveOccurred())
		Expect(verified).To(BeFalse())
		Expect(message).To(Equal(fmt.Sprintf("DaemonSet %s, image kernel version %s different from upgrade kernel version %s", dsName, incorrectUpgradeKernelVersion, upgradeKernelVersion)))
	})

	It("dtk is missing", func() {
		digestsList := []string{firstDigestLayer}
		digestLayer := v1stream.Layer{}
		daemonObj := prepareDaemonSet("driver-module")

		mockRegistryAPI.EXPECT().GetLayersDigests(gomock.Any(), dsImage).Return(layersRepo, digestsList, nil, nil)
		mockRegistryAPI.EXPECT().GetLayerByDigest(layersRepo, firstDigestLayer, nil).Return(&digestLayer, nil)
		mockRegistryAPI.EXPECT().ExtractToolkitRelease(&digestLayer).Return(nil, fmt.Errorf("some error"))

		verified, message, err := p.daemonSetPreflightCheck(context.TODO(), daemonObj, upgradeKernelVersion)

		Expect(err).NotTo(HaveOccurred())
		Expect(verified).To(BeFalse())
		Expect(message).To(Equal(fmt.Sprintf("DaemonSet %s, image %s does not contain DTK data on any layer", dsName, dsImage)))
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

// numBuildConfigs should less or equal to numDaemonSets
// all BuildConfigs should paired to DaemonSets
func prepareObjListForTest(numIrrelevantItems int,
	numBuildConfigs int,
	numDaemonSets int) *unstructured.UnstructuredList {
	Expect(numBuildConfigs).To(BeNumerically("<=", numDaemonSets))
	objList := &unstructured.UnstructuredList{
		Items: []unstructured.Unstructured{},
	}

	for i := 0; i < numIrrelevantItems; i++ {
		item := unstructured.Unstructured{}
		item.SetKind(fmt.Sprintf("objKind%d", i))
		objList.Items = append(objList.Items, item)
	}
	for i := 0; i < numBuildConfigs; i++ {
		buildItem := unstructured.Unstructured{}
		buildItem.SetKind("BuildConfig")
		annotatations := map[string]string{
			"specialresource.openshift.io/driver-container-vendor": fmt.Sprintf("driver-module-with-build-config%d", i),
		}
		buildItem.SetAnnotations(annotatations)
		objList.Items = append(objList.Items, buildItem)
	}
	for i := 0; i < numDaemonSets; i++ {
		moduleName := fmt.Sprintf("driver-module-with-build-config%d", i)
		if i >= numBuildConfigs {
			moduleName = fmt.Sprintf("driver-module-without-build-config%d", i)
		}
		objList.Items = append(objList.Items, *prepareDaemonSetObj(moduleName))
	}

	/*
		if buildConfigFlag {
			buildItem := unstructured.Unstructured{}
			buildItem.SetKind("BuildConfig")
			objList.Items = append(objList.Items, buildItem)
		}
		if daemonSetFlag {
			objList.Items = append(objList.Items, *prepareDaemonSetObj())
		}
	*/
	return objList
}

func prepareDaemonSetObj(moduleName string) *unstructured.Unstructured {
	annotations := map[string]string{
		"specialresource.openshift.io/state":                   "driver-container",
		"specialresource.openshift.io/driver-container-vendor": moduleName,
	}
	item := unstructured.Unstructured{}
	item.SetKind("DaemonSet")
	item.SetName(dsName)
	item.SetAnnotations(annotations)
	containersSlice := make([]interface{}, 1)
	imageSlice := map[string]interface{}{}
	containersSlice[0] = imageSlice
	err := unstructured.SetNestedField(imageSlice, dsImage, "image")
	Expect(err).NotTo(HaveOccurred())
	err = unstructured.SetNestedSlice(item.Object, containersSlice, "spec", "template", "spec", "containers")
	Expect(err).NotTo(HaveOccurred())
	return &item
}

func prepareDaemonSet(moduleName string) *k8sv1.DaemonSet {
	ds := k8sv1.DaemonSet{}
	obj := prepareDaemonSetObj(moduleName)
	err := k8sruntime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &ds)
	Expect(err).NotTo(HaveOccurred())
	return &ds
}
