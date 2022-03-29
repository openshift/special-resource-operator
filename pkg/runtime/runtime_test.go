package runtime

import (
	"context"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	srov1beta1 "github.com/openshift/special-resource-operator/api/v1beta1"
	v1 "k8s.io/api/core/v1"

	"github.com/openshift/special-resource-operator/pkg/clients"
	"github.com/openshift/special-resource-operator/pkg/cluster"
	"github.com/openshift/special-resource-operator/pkg/kernel"
	"github.com/openshift/special-resource-operator/pkg/proxy"
	"github.com/openshift/special-resource-operator/pkg/upgrade"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestPkgRutime(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Runtime Suite")
}

var _ = Describe("getPushSecretName", func() {
	var (
		mockCtrl        *gomock.Controller
		mockKubeClient  *clients.MockClientsInterface
		mockCluster     *cluster.MockCluster
		mockKernel      *kernel.MockKernelData
		mockClusterInfo *upgrade.MockClusterInfo
		mockProxy       *proxy.MockProxyAPI
		runtimeStruct   *runtime
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = clients.NewMockClientsInterface(mockCtrl)
		mockCluster = cluster.NewMockCluster(mockCtrl)
		mockKernel = kernel.NewMockKernelData(mockCtrl)
		mockClusterInfo = upgrade.NewMockClusterInfo(mockCtrl)
		mockProxy = proxy.NewMockProxyAPI(mockCtrl)

		runtimeStruct = &runtime{
			log:            zap.New(zap.WriteTo(ioutil.Discard)),
			kubeClient:     mockKubeClient,
			clusterAPI:     mockCluster,
			kernelAPI:      mockKernel,
			clusterInfoAPI: mockClusterInfo,
			proxyAPI:       mockProxy,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("kube client list fails", func() {
		sr := &srov1beta1.SpecialResource{}
		mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(fmt.Errorf("some error"))
		res, err := runtimeStruct.getPushSecretName(context.TODO(), sr, "whatever")
		Expect(err).To(HaveOccurred())
		Expect(res).To(BeEmpty())
	})

	It("kube client list good flow", func() {
		sr := &srov1beta1.SpecialResource{}
		sr.Spec.Namespace = "my_namespace"
		secrets := &v1.SecretList{}

		optNs := client.InNamespace(sr.Spec.Namespace)
		mockKubeClient.EXPECT().List(context.TODO(), secrets, optNs).
			DoAndReturn(func(_ context.Context, secrets *v1.SecretList, _ client.ListOption) error {
				item1 := v1.Secret{}
				item1.SetName("builder-dockercfg")
				secrets.Items = []v1.Secret{}
				secrets.Items = append(secrets.Items, item1)
				return nil
			})
		res, err := runtimeStruct.getPushSecretName(context.TODO(), sr, "whatever")
		Expect(err).ToNot(HaveOccurred())
		Expect(res).To(Equal("builder-dockercfg"))
	})
})

var _ = Describe("GetRuntimeInformation", func() {
	var (
		mockCtrl        *gomock.Controller
		mockKubeClient  *clients.MockClientsInterface
		mockCluster     *cluster.MockCluster
		mockKernel      *kernel.MockKernelData
		mockClusterInfo *upgrade.MockClusterInfo
		mockProxy       *proxy.MockProxyAPI
		runtimeStruct   *runtime
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = clients.NewMockClientsInterface(mockCtrl)
		mockCluster = cluster.NewMockCluster(mockCtrl)
		mockKernel = kernel.NewMockKernelData(mockCtrl)
		mockClusterInfo = upgrade.NewMockClusterInfo(mockCtrl)
		mockProxy = proxy.NewMockProxyAPI(mockCtrl)

		runtimeStruct = &runtime{
			log:            zap.New(zap.WriteTo(ioutil.Discard)),
			kubeClient:     mockKubeClient,
			clusterAPI:     mockCluster,
			kernelAPI:      mockKernel,
			clusterInfoAPI: mockClusterInfo,
			proxyAPI:       mockProxy,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("good flow", func() {
		sr := &srov1beta1.SpecialResource{}
		sr.Spec.Namespace = "my_namespace"
		sr.Spec.NodeSelector = map[string]string{"key": "value"}
		nodeList := v1.NodeList{
			Items: []v1.Node{
				{},
			},
		}
		secrets := &v1.SecretList{}
		optNs := client.InNamespace(sr.Spec.Namespace)

		osMajor := "osMajor"
		osMajorMinor := "osMajorMinor"
		osDecimal := "osDecimal"
		kernelFullVersion := "kernelFullVersion"
		kernelPatchVersion := "kernelPatchVersion"
		clusterVersion := "clusterVersion"
		clusterVersionMajorMinor := "clusterMajorMinor"
		clusterUpgradeInfo := map[string]upgrade.NodeVersion{"key": {}}
		osImageURL := "osImageURL"
		proxyConfiguration := proxy.Configuration{}

		mockKubeClient.EXPECT().GetNodesByLabels(gomock.Any(), sr.Spec.NodeSelector).Return(&nodeList, nil)
		mockCluster.EXPECT().OperatingSystem(&nodeList).Return(osMajor, osMajorMinor, osDecimal, nil)
		mockKernel.EXPECT().FullVersion(&nodeList).Return(kernelFullVersion, nil)
		mockKernel.EXPECT().PatchVersion(kernelFullVersion).Return(kernelPatchVersion, nil)
		mockCluster.EXPECT().Version(gomock.Any()).Return(clusterVersion, clusterVersionMajorMinor, nil)
		mockClusterInfo.EXPECT().GetClusterInfo(gomock.Any(), &nodeList).Return(clusterUpgradeInfo, nil)
		mockKubeClient.EXPECT().List(context.TODO(), secrets, optNs).
			DoAndReturn(func(_ context.Context, secrets *v1.SecretList, _ client.ListOption) error {
				item1 := v1.Secret{}
				item1.SetName("builder-dockercfg")
				secrets.Items = []v1.Secret{}
				secrets.Items = append(secrets.Items, item1)
				return nil
			})
		mockCluster.EXPECT().OSImageURL(gomock.Any()).Return(osImageURL, nil)
		mockProxy.EXPECT().ClusterConfiguration(gomock.Any()).Return(proxyConfiguration, nil)

		runInfo, err := runtimeStruct.GetRuntimeInformation(context.TODO(), sr)
		Expect(err).ToNot(HaveOccurred())
		Expect(runInfo.OperatingSystemMajor).To(Equal(osMajor))
		Expect(runInfo.OperatingSystemMajorMinor).To(Equal(osMajorMinor))
		Expect(runInfo.OperatingSystemDecimal).To(Equal(osDecimal))
		Expect(runInfo.KernelFullVersion).To(Equal(kernelFullVersion))
		Expect(runInfo.KernelPatchVersion).To(Equal(kernelPatchVersion))
		Expect(runInfo.Platform).To(Equal("OCP"))
		Expect(runInfo.ClusterVersion).To(Equal(clusterVersion))
		Expect(runInfo.ClusterVersionMajorMinor).To(Equal(clusterVersionMajorMinor))
		Expect(runInfo.ClusterUpgradeInfo).To(Equal(clusterUpgradeInfo))
		Expect(runInfo.PushSecretName).To(Equal("builder-dockercfg"))
		Expect(runInfo.OSImageURL).To(Equal(osImageURL))
		Expect(runInfo.Proxy).To(Equal(proxyConfiguration))
	})
})
