package runtime

import (
	"context"
	"fmt"
	"strings"

	srov1beta1 "github.com/openshift-psap/special-resource-operator/api/v1beta1"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/cluster"
	"github.com/openshift-psap/special-resource-operator/pkg/kernel"
	"github.com/openshift-psap/special-resource-operator/pkg/proxy"
	"github.com/openshift-psap/special-resource-operator/pkg/upgrade"
	"github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	GetRuntimeInformation(ctx context.Context, sr *srov1beta1.SpecialResource) (*RuntimeInformation, error)
	LogRuntimeInformation(ctx context.Context, info *RuntimeInformation)
}

type runtime struct {
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
		kubeClient:     kubeClient,
		clusterAPI:     clusterAPI,
		kernelAPI:      kernelAPI,
		clusterInfoAPI: clusterInfoAPI,
		proxyAPI:       proxyAPI,
	}
}

func (rt *runtime) LogRuntimeInformation(ctx context.Context, info *RuntimeInformation) {
	ctrl.LoggerFrom(ctx).Info("Runtime Information",
		"OperatingSystemMajor", info.OperatingSystemMajor,
		"OperatingSystemMajorMinor", info.OperatingSystemMajorMinor,
		"OperatingSystemDecimal", info.OperatingSystemDecimal,
		"KernelFullVersion", info.KernelFullVersion,
		"KernelPatchVersion", info.KernelPatchVersion,
		"DriverToolkitImage", info.DriverToolkitImage,
		"Platform", info.Platform,
		"ClusterVersion", info.ClusterVersion,
		"ClusterVersionMajorMinor", info.ClusterVersionMajorMinor,
		"ClusterUpgradeInfo", info.ClusterUpgradeInfo,
		"PushSecretName", info.PushSecretName,
		"OSImageURL", info.OSImageURL,
		"Proxy", info.Proxy)
}

func (rt *runtime) GetRuntimeInformation(ctx context.Context, sr *srov1beta1.SpecialResource) (*RuntimeInformation, error) {

	info := &RuntimeInformation{
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
	}

	nodeList, err := rt.kubeClient.GetNodesByLabels(ctx, sr.Spec.NodeSelector)
	if err != nil {
		return nil, fmt.Errorf("failed to get nodes list during getRuntimeInformation: %w", err)
	}

	info.OperatingSystemMajor, info.OperatingSystemMajorMinor, info.OperatingSystemDecimal, err = rt.clusterAPI.OperatingSystem(nodeList)
	if err != nil {
		return nil, fmt.Errorf("failed to get operating system: %w", err)
	}

	info.KernelFullVersion, err = rt.kernelAPI.FullVersion(nodeList)
	if err != nil {
		return nil, fmt.Errorf("failed to get kernel version: %w", err)
	}

	info.KernelPatchVersion, err = rt.kernelAPI.PatchVersion(info.KernelFullVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to get kernel patch version: %w", err)
	}

	// Only want to initialize the platform once.
	if info.Platform == "" {
		info.Platform, err = rt.kubeClient.GetPlatform()
		if err != nil {
			return nil, fmt.Errorf("failed to determine platform: %v", err)
		}
	}

	info.ClusterVersion, info.ClusterVersionMajorMinor, err = rt.clusterAPI.Version(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster version: %w", err)
	}

	info.ClusterUpgradeInfo, err = rt.clusterInfoAPI.GetClusterInfo(ctx, nodeList)
	if err != nil {
		return nil, fmt.Errorf("failed to get upgrade info: %w", err)
	}

	info.PushSecretName, err = rt.getPushSecretName(ctx, sr, info.Platform)
	if err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "Failed to get push secret's name")
	}

	info.OSImageURL, err = rt.clusterAPI.OSImageURL(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get OSImageURL: %w", err)
	}

	info.Proxy, err = rt.proxyAPI.ClusterConfiguration(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get Proxy Configuration: %w", err)
	}

	sr.DeepCopyInto(&info.SpecialResource)

	return info, nil
}

func (rt *runtime) getPushSecretName(ctx context.Context, sr *srov1beta1.SpecialResource, platform string) (string, error) {
	secrets := &corev1.SecretList{}
	err := rt.kubeClient.List(ctx, secrets, client.InNamespace(sr.Spec.Namespace))
	if err != nil {
		return "", errors.Wrap(err, "cannot get SecretList")
	}
	for _, secret := range secrets.Items {
		secretName := secret.GetName()
		if strings.Contains(secretName, "builder-dockercfg") {
			return secretName, nil
		}
	}
	return "", errors.New("cannot find Secret builder-dockercfg")
}
