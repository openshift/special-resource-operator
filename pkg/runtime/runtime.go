package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	srov1beta1 "github.com/openshift-psap/special-resource-operator/api/v1beta1"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/cluster"
	"github.com/openshift-psap/special-resource-operator/pkg/kernel"
	"github.com/openshift-psap/special-resource-operator/pkg/proxy"
	"github.com/openshift-psap/special-resource-operator/pkg/upgrade"
	"github.com/openshift-psap/special-resource-operator/pkg/utils"
	"github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type ResourceGroupName struct {
	DriverBuild            string `json:"driverBuild"`
	DriverContainer        string `json:"driverContainer"`
	RuntimeEnablement      string `json:"runtimeEnablement"`
	DevicePlugin           string `json:"devicePlugin"`
	DeviceMonitoring       string `json:"deviceMonitoring"`
	DeviceDashboard        string `json:"deviceDashboard"`
	DeviceFeatureDiscovery string `json:"deviceFeatureDiscovery"`
	CSIDriver              string `json:"csiDriver"`
}

type RuntimeInformation struct {
	Kind                      string                         `json:"kind"`
	OperatingSystemMajor      string                         `json:"operatingSystemMajor"`
	OperatingSystemMajorMinor string                         `json:"operatingSystemMajorMinor"`
	OperatingSystemDecimal    string                         `json:"operatingSystemDecimal"`
	KernelFullVersion         string                         `json:"kernelFullVersion"`
	KernelPatchVersion        string                         `json:"kernelPatchVersion"`
	DriverToolkitImage        string                         `json:"driverToolkitImage"`
	Platform                  string                         `json:"platform"`
	ClusterVersion            string                         `json:"clusterVersion"`
	ClusterVersionMajorMinor  string                         `json:"clusterVersionMajorMinor"`
	ClusterUpgradeInfo        map[string]upgrade.NodeVersion `json:"clusterUpgradeInfo"`
	PushSecretName            string                         `json:"pushSecretName"`
	OSImageURL                string                         `json:"osImageURL"`
	Proxy                     proxy.Configuration            `json:"proxy"`
	GroupName                 ResourceGroupName              `json:"groupName"`
	SpecialResource           srov1beta1.SpecialResource     `json:"specialresource"`
}

//go:generate mockgen -source=runtime.go -package=runtime -destination=mock_runtime_api.go

type RuntimeAPI interface {
	GetRuntimeInformation(ctx context.Context, sr *srov1beta1.SpecialResource, runInfo *RuntimeInformation) error
	LogRuntimeInformation(runInfo *RuntimeInformation)
	InitRunInfo() RuntimeInformation
}

type runtime struct {
	log            logr.Logger
	kubeClient     clients.ClientsInterface
	clusterAPI     cluster.Cluster
	kernelAPI      kernel.KernelData
	clusterInfoAPI upgrade.ClusterInfo
	proxyAPI       proxy.ProxyAPI
}

func NewRuntimeAPI(kubeClient clients.ClientsInterface,
	clusterAPI cluster.Cluster,
	kernelAPI kernel.KernelData,
	clusterInfoAPI upgrade.ClusterInfo,
	proxyAPI proxy.ProxyAPI) RuntimeAPI {
	return &runtime{
		log:            zap.New(zap.UseDevMode(true)).WithName(utils.Print("runtime", utils.Blue)),
		kubeClient:     kubeClient,
		clusterAPI:     clusterAPI,
		kernelAPI:      kernelAPI,
		clusterInfoAPI: clusterInfoAPI,
		proxyAPI:       proxyAPI,
	}
}

func (rt *runtime) InitRunInfo() RuntimeInformation {
	return RuntimeInformation{
		Kind:                      "Values",
		OperatingSystemMajor:      "",
		OperatingSystemMajorMinor: "",
		OperatingSystemDecimal:    "",
		KernelFullVersion:         "",
		KernelPatchVersion:        "",
		DriverToolkitImage:        "",
		Platform:                  "",
		ClusterVersion:            "",
		ClusterVersionMajorMinor:  "",
		ClusterUpgradeInfo:        make(map[string]upgrade.NodeVersion),
		PushSecretName:            "",
		OSImageURL:                "",
		Proxy:                     proxy.Configuration{},
		GroupName:                 ResourceGroupName{DriverBuild: "driver-build", DriverContainer: "driver-container", RuntimeEnablement: "runtime-enablement", DevicePlugin: "device-plugin", DeviceMonitoring: "device-monitoring", DeviceDashboard: "device-dashboard", DeviceFeatureDiscovery: "device-feature-discovery", CSIDriver: "csi-driver"},
		SpecialResource:           srov1beta1.SpecialResource{},
	}
}

func (rt *runtime) LogRuntimeInformation(runInfo *RuntimeInformation) {
	rt.log.Info("Runtime Information",
		"OperatingSystemMajor", runInfo.OperatingSystemMajor,
		"OperatingSystemMajorMinor", runInfo.OperatingSystemMajorMinor,
		"OperatingSystemDecimal", runInfo.OperatingSystemDecimal,
		"KernelFullVersion", runInfo.KernelFullVersion,
		"KernelPatchVersion", runInfo.KernelPatchVersion,
		"DriverToolkitImage", runInfo.DriverToolkitImage,
		"Platform", runInfo.Platform,
		"ClusterVersion", runInfo.ClusterVersion,
		"ClusterVersionMajorMinor", runInfo.ClusterVersionMajorMinor,
		"ClusterUpgradeInfo", runInfo.ClusterUpgradeInfo,
		"PushSecretName", runInfo.PushSecretName,
		"OSImageURL", runInfo.OSImageURL,
		"Proxy", runInfo.Proxy)
}

func (rt *runtime) GetRuntimeInformation(ctx context.Context, sr *srov1beta1.SpecialResource, runInfo *RuntimeInformation) error {
	nodeList, err := rt.kubeClient.GetNodesByLabels(ctx, sr.Spec.NodeSelector)
	if err != nil {
		return fmt.Errorf("failed to get nodes list during getRuntimeInformation: %w", err)
	}

	runInfo.OperatingSystemMajor, runInfo.OperatingSystemMajorMinor, runInfo.OperatingSystemDecimal, err = rt.clusterAPI.OperatingSystem(nodeList)
	if err != nil {
		return fmt.Errorf("failed to get operating system: %w", err)
	}

	runInfo.KernelFullVersion, err = rt.kernelAPI.FullVersion(nodeList)
	if err != nil {
		return fmt.Errorf("failed to get kernel version: %w", err)
	}

	runInfo.KernelPatchVersion, err = rt.kernelAPI.PatchVersion(runInfo.KernelFullVersion)
	if err != nil {
		return fmt.Errorf("failed to get kernel patch version: %w", err)
	}

	// Only want to initialize the platform once.
	if runInfo.Platform == "" {
		runInfo.Platform, err = rt.kubeClient.GetPlatform()
		if err != nil {
			return fmt.Errorf("failed to determine platform: %v", err)
		}
	}

	runInfo.ClusterVersion, runInfo.ClusterVersionMajorMinor, err = rt.clusterAPI.Version(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cluster version: %w", err)
	}

	runInfo.ClusterUpgradeInfo, err = rt.clusterInfoAPI.GetClusterInfo(ctx, nodeList)
	if err != nil {
		return fmt.Errorf("failed to get upgrade info: %w", err)
	}

	runInfo.PushSecretName, err = rt.getPushSecretName(ctx, sr, runInfo.Platform)
	utils.WarnOnError(err)

	runInfo.OSImageURL, err = rt.clusterAPI.OSImageURL(ctx)
	if err != nil {
		return fmt.Errorf("failed to get OSImageURL: %w", err)
	}

	runInfo.Proxy, err = rt.proxyAPI.ClusterConfiguration(ctx)
	if err != nil {
		return fmt.Errorf("failed to get Proxy Configuration: %w", err)
	}

	sr.DeepCopyInto(&runInfo.SpecialResource)

	return nil
}

func (rt *runtime) getPushSecretName(ctx context.Context, sr *srov1beta1.SpecialResource, platform string) (string, error) {
	secrets := &corev1.SecretList{}

	rt.log.Info("Getting SecretList in Namespace: " + sr.Spec.Namespace)
	err := rt.kubeClient.List(ctx, secrets, client.InNamespace(sr.Spec.Namespace))
	if err != nil {
		return "", errors.Wrap(err, "Client cannot get SecretList")
	}

	rt.log.Info("Searching for builder-dockercfg Secret")
	for _, secret := range secrets.Items {
		secretName := secret.GetName()

		if strings.Contains(secretName, "builder-dockercfg") {
			rt.log.Info("Found", "Secret", secretName)
			return secretName, nil
		}
	}

	return "", errors.New("cannot find Secret builder-dockercfg")
}
