package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	srov1beta1 "github.com/openshift-psap/special-resource-operator/api/v1beta1"
	"github.com/openshift-psap/special-resource-operator/pkg/cache"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/proxy"
	"github.com/openshift-psap/special-resource-operator/pkg/upgrade"
	"github.com/openshift-psap/special-resource-operator/pkg/utils"
	"github.com/pkg/errors"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

var RunInfo = RuntimeInformation{
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

func logRuntimeInformation() {
	log.Info("Runtime Information", "OperatingSystemMajor", RunInfo.OperatingSystemMajor)
	log.Info("Runtime Information", "OperatingSystemMajorMinor", RunInfo.OperatingSystemMajorMinor)
	log.Info("Runtime Information", "OperatingSystemDecimal", RunInfo.OperatingSystemDecimal)
	log.Info("Runtime Information", "KernelFullVersion", RunInfo.KernelFullVersion)
	log.Info("Runtime Information", "KernelPatchVersion", RunInfo.KernelPatchVersion)
	log.Info("Runtime Information", "DriverToolkitImage", RunInfo.DriverToolkitImage)
	log.Info("Runtime Information", "Platform", RunInfo.Platform)
	log.Info("Runtime Information", "ClusterVersion", RunInfo.ClusterVersion)
	log.Info("Runtime Information", "ClusterVersionMajorMinor", RunInfo.ClusterVersionMajorMinor)
	log.Info("Runtime Information", "ClusterUpgradeInfo", RunInfo.ClusterUpgradeInfo)
	log.Info("Runtime Information", "PushSecretName", RunInfo.PushSecretName)
	log.Info("Runtime Information", "OSImageURL", RunInfo.OSImageURL)
	log.Info("Runtime Information", "Proxy", RunInfo.Proxy)
}

func getRuntimeInformation(ctx context.Context, r *SpecialResourceReconciler) error {
	var err error

	if err = cache.Nodes(ctx, r.specialresource.Spec.NodeSelector, false); err != nil {
		return fmt.Errorf("failed to cache nodes: %w", err)
	}

	RunInfo.OperatingSystemMajor, RunInfo.OperatingSystemMajorMinor, RunInfo.OperatingSystemDecimal, err = r.Cluster.OperatingSystem()
	if err != nil {
		return fmt.Errorf("failed to get operating system: %w", err)
	}

	RunInfo.KernelFullVersion, err = r.KernelData.FullVersion()
	if err != nil {
		return fmt.Errorf("failed to get kernel version: %w", err)
	}

	RunInfo.KernelPatchVersion, err = r.KernelData.PatchVersion(RunInfo.KernelFullVersion)
	if err != nil {
		return fmt.Errorf("failed to get kernel patch version: %w", err)
	}

	// Only want to initialize the platform once.
	if RunInfo.Platform == "" {
		RunInfo.Platform, err = clients.Interface.GetPlatform()
		if err != nil {
			return fmt.Errorf("failed to determine platform: %v", err)
		}
	}

	RunInfo.ClusterVersion, RunInfo.ClusterVersionMajorMinor, err = r.Cluster.Version(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cluster version: %w", err)
	}

	RunInfo.ClusterUpgradeInfo, err = r.ClusterInfo.GetClusterInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to get upgrade info: %w", err)
	}

	RunInfo.PushSecretName, err = retryGetPushSecretName(ctx, r)
	utils.WarnOnError(err)

	RunInfo.OSImageURL, err = r.Cluster.OSImageURL(ctx)
	if err != nil {
		return fmt.Errorf("failed to get OSImageURL: %w", err)
	}

	RunInfo.Proxy, err = r.ProxyAPI.ClusterConfiguration(ctx)
	if err != nil {
		return fmt.Errorf("failed to get Proxy Configuration: %w", err)
	}

	r.specialresource.DeepCopyInto(&RunInfo.SpecialResource)

	return nil
}

func retryGetPushSecretName(ctx context.Context, r *SpecialResourceReconciler) (string, error) {
	for i := 0; i < 3; i++ {
		time.Sleep(2 * time.Second)
		pushSecretName, err := getPushSecretName(ctx, r)
		if err != nil {
			log.Info("Cannot find Secret builder-dockercfg " + r.specialresource.Spec.Namespace)
			continue
		} else {
			return pushSecretName, err
		}
	}

	return "", errors.New("Cannot find Secret builder-dockercfg")

}

func getPushSecretName(ctx context.Context, r *SpecialResourceReconciler) (string, error) {
	if RunInfo.Platform == "K8S" {
		log.Info("Warning: On vanilla K8s. Skipping search for push-secret")
		return "", nil
	}

	secrets := &unstructured.UnstructuredList{}

	secrets.SetAPIVersion("v1")
	secrets.SetKind("SecretList")

	log.Info("Getting SecretList in Namespace: " + r.specialresource.Spec.Namespace)
	opts := []client.ListOption{
		client.InNamespace(r.specialresource.Spec.Namespace),
	}
	err := clients.Interface.List(ctx, secrets, opts...)
	if err != nil {
		return "", errors.Wrap(err, "Client cannot get SecretList")
	}

	log.Info("Searching for builder-dockercfg Secret")
	for _, secret := range secrets.Items {
		secretName := secret.GetName()

		if strings.Contains(secretName, "builder-dockercfg") {
			log.Info("Found", "Secret", secretName)
			return secretName, nil
		}
	}

	return "", errors.New("Cannot find Secret builder-dockercfg")
}
