package preflight

import (
	"context"
	"fmt"

	srov1beta1 "github.com/openshift/special-resource-operator/api/v1beta1"
	"github.com/openshift/special-resource-operator/pkg/cluster"
	"github.com/openshift/special-resource-operator/pkg/helmer"
	"github.com/openshift/special-resource-operator/pkg/kernel"
	"github.com/openshift/special-resource-operator/pkg/registry"
	"github.com/openshift/special-resource-operator/pkg/resource"
	"github.com/openshift/special-resource-operator/pkg/runtime"
	"github.com/openshift/special-resource-operator/pkg/upgrade"
	"github.com/openshift/special-resource-operator/pkg/utils"
	helmchart "helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	k8sv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	ctrlruntime "sigs.k8s.io/controller-runtime"
)

const (
	VerificationStatusReasonBuildConfigPresent = "Verification successful, all driver-containers have paired BuildConfigs in the recipe"
	VerificationStatusReasonNoDaemonSet        = "Verification successful, no driver-container present in the recipe"
	VerificationStatusReasonUnknown            = "Verification has not started yet"
	VerificationStatusReasonVerified           = "Verification successful, all driver-containers for the next kernel version are present"
)

//go:generate mockgen -source=preflight.go -package=preflight -destination=mock_preflight_api.go

type PreflightAPI interface {
	PreflightUpgradeCheck(ctx context.Context,
		sr *srov1beta1.SpecialResource,
		runInfo *runtime.RuntimeInformation) (bool, string, error)
	PrepareRuntimeInfo(ctx context.Context, image string) (*runtime.RuntimeInformation, error)
}

func NewPreflightAPI(registryAPI registry.Registry,
	clusterAPI cluster.Cluster,
	clusterInfoAPI upgrade.ClusterInfo,
	resourceAPI resource.ResourceAPI,
	helmerAPI helmer.Helmer,
	runtimeAPI runtime.RuntimeAPI,
	kernelAPI kernel.KernelData) PreflightAPI {
	return &preflight{
		registryAPI:    registryAPI,
		clusterAPI:     clusterAPI,
		clusterInfoAPI: clusterInfoAPI,
		resourceAPI:    resourceAPI,
		helmerAPI:      helmerAPI,
		runtimeAPI:     runtimeAPI,
		kernelAPI:      kernelAPI,
	}
}

type preflight struct {
	registryAPI    registry.Registry
	clusterAPI     cluster.Cluster
	clusterInfoAPI upgrade.ClusterInfo
	resourceAPI    resource.ResourceAPI
	helmerAPI      helmer.Helmer
	runtimeAPI     runtime.RuntimeAPI
	kernelAPI      kernel.KernelData
}

func (p *preflight) PreflightUpgradeCheck(ctx context.Context,
	sr *srov1beta1.SpecialResource,
	runInfo *runtime.RuntimeInformation) (bool, string, error) {

	sr.DeepCopyInto(&runInfo.SpecialResource)

	chart, err := p.helmerAPI.Load(sr.Spec.Chart)
	if err != nil {
		err = fmt.Errorf("failed to load chart in PreflightUpgradeCheck, CR %s: %w", sr.Name, err)
		return false, fmt.Sprintf("Failed to load helm chart for CR %s", sr.Name), err
	}

	yamlsList, err := p.processFullChartTemplates(ctx, chart, sr.Spec.Set, runInfo, sr.Namespace, runInfo.KernelFullVersion)
	if err != nil {
		err = fmt.Errorf("failed to processFullChartTemplates in PreflightUpgradeCheck, CR name %s: %w", sr.Name, err)
		return false, fmt.Sprintf("Failed to process full chart for CR %s", sr.Name), err
	}
	return p.handleYAMLsCheck(ctx, yamlsList, runInfo.KernelFullVersion)
}

func (p *preflight) processFullChartTemplates(ctx context.Context,
	chart *helmchart.Chart,
	values unstructured.Unstructured,
	runInfo *runtime.RuntimeInformation,
	namespace string,
	upgradeKernelVersion string) (string, error) {

	var err error
	// [TODO] - check that templates are not messed up during helm processing
	fullChart := *chart
	fullChart.Templates = []*helmchart.File{}
	fullChart.Templates = append(fullChart.Templates, chart.Templates...)

	nodeVersion := runInfo.ClusterUpgradeInfo[upgradeKernelVersion]

	runInfo.ClusterVersionMajorMinor = nodeVersion.ClusterVersion
	runInfo.OperatingSystemDecimal = nodeVersion.OSVersion
	runInfo.OperatingSystemMajorMinor = nodeVersion.OSMajorMinor
	runInfo.OperatingSystemMajor = nodeVersion.OSMajor
	runInfo.DriverToolkitImage = nodeVersion.DriverToolkit.ImageURL

	fullChart.Values, err = chartutil.CoalesceValues(&fullChart, values.Object)
	if err != nil {
		return "", fmt.Errorf("failed to coalesce values in processFullChartTemplates, chart name %s: %w", fullChart.Name(), err)
	}
	rinfo, err := k8sruntime.DefaultUnstructuredConverter.ToUnstructured(&runInfo)
	if err != nil {
		return "", fmt.Errorf("failed to onvert runInfo type to unstructured in processFullChartTemplates, chart name %s: %w", fullChart.Name(), err)
	}

	fullChart.Values, err = chartutil.CoalesceValues(&fullChart, rinfo)
	if err != nil {
		return "", fmt.Errorf("failed to coalesce runInfo into chart in processFullChartTemplates, chart name %s: %w", fullChart.Name(), err)
	}

	return p.helmerAPI.GetHelmOutput(ctx, fullChart, fullChart.Values, namespace)
}

func (p *preflight) handleYAMLsCheck(ctx context.Context, yamlsList string, upgradeKernelVersion string) (bool, string, error) {
	objList, err := p.resourceAPI.GetObjectsFromYAML([]byte(yamlsList))
	if err != nil {
		err = fmt.Errorf("failed to extract object from chart yaml listing handleYAMLsCheck: %w", err)
		return false, "Failed to extract object from chart yaml list during preflight", err
	}

	daemonSetList, buildConfigPresent, err := p.getRelevantDaemonSets(objList)
	if err != nil {
		err = fmt.Errorf("failure of getRelevantDaemonSets in handleYAMLsCheck: %w", err)
		return false, "Error while trying to parse BuildConfigs and DaemonSets", err
	}
	if len(daemonSetList) == 0 {
		if buildConfigPresent {
			return true, VerificationStatusReasonBuildConfigPresent, nil
		} else {
			return true, VerificationStatusReasonNoDaemonSet, nil
		}
	}

	for _, daemonSet := range daemonSetList {
		verified, message, err := p.daemonSetPreflightCheck(ctx, daemonSet, upgradeKernelVersion)
		if err != nil {
			return false, message, fmt.Errorf("failure of daemonSet %s in handleYAMLsCheck: %w", daemonSet.Name, err)
		}
		if !verified {
			return verified, message, nil
		}
	}
	return true, VerificationStatusReasonVerified, nil
}

func (p *preflight) getRelevantDaemonSets(objList *unstructured.UnstructuredList) ([]*k8sv1.DaemonSet, bool, error) {
	buildConfigMap := map[string]struct{}{}
	tempDSList := []*k8sv1.DaemonSet{}
	buildConfigPresent := false
	for _, obj := range objList.Items {
		switch obj.GetKind() {
		case "BuildConfig":
			buildConfigPresent = true
			annotations := obj.GetAnnotations()
			driverContainerToken, found := annotations["specialresource.openshift.io/driver-container-vendor"]
			if found {
				buildConfigMap[driverContainerToken] = struct{}{}
			}

		case "DaemonSet":
			daemonSet := k8sv1.DaemonSet{}
			err := k8sruntime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &daemonSet)
			if err != nil {
				return nil, false, fmt.Errorf("failed to convert unstructured into DaemonSet in getRelevantDaemonSets: %w", err)
			}
			state, found := daemonSet.Annotations["specialresource.openshift.io/state"]
			if found && state == "driver-container" {
				tempDSList = append(tempDSList, &daemonSet)
			}
		}
	}
	resultList := []*k8sv1.DaemonSet{}
	for _, item := range tempDSList {
		driverContainerToken := item.Annotations["specialresource.openshift.io/driver-container-vendor"]
		if _, exists := buildConfigMap[driverContainerToken]; !exists {
			resultList = append(resultList, item)
		}
	}
	return resultList, buildConfigPresent, nil
}

func (p *preflight) daemonSetPreflightCheck(ctx context.Context, ds *k8sv1.DaemonSet, upgradeKernelVersion string) (bool, string, error) {
	log := ctrlruntime.LoggerFrom(ctx)
	if len(ds.Spec.Template.Spec.Containers) == 0 {
		return false, fmt.Sprintf("invalid daemonset %s, no container  data present", ds.Name), fmt.Errorf("invalid daemonset %s, no container  data present", ds.Name)
	}
	image := ds.Spec.Template.Spec.Containers[0].Image

	repo, digests, auth, err := p.registryAPI.GetLayersDigests(ctx, image)
	if err != nil {
		log.Info("image layers inaccessible, DS image probably does not exists", "daemonset", ds.Name, "image", image)
		return false, fmt.Sprintf("DaemonSet %s, image %s inaccessible or does not exists", ds.Name, image), nil
	}

	for i := len(digests) - 1; i >= 0; i-- {
		layer, err := p.registryAPI.GetLayerByDigest(repo, digests[i], auth)
		if err != nil {
			log.Info("layer from image inaccessible", "layer", digests[i], "repo", repo, "image", image)
			return false, fmt.Sprintf("DaemonSet %s, image %s, layer %s is inaccessible", ds.Name, image, digests[i]), nil
		}
		dtk, err := p.registryAPI.ExtractToolkitRelease(layer)
		if err != nil {
			continue
		}

		if dtk.KernelFullVersion != upgradeKernelVersion {
			log.Info("DTK kernel version differs from the upgrade node version", "ds name", ds.Name, "dtkVersion", dtk.KernelFullVersion, "upgradeVersion", upgradeKernelVersion)
			return false, fmt.Sprintf("DaemonSet %s, image kernel version %s different from upgrade kernel version %s", ds.Name, dtk.KernelFullVersion, upgradeKernelVersion), nil
		}
		return true, VerificationStatusReasonVerified, nil
	}

	log.Info("DTK info not present on any layer of the image, invaid image format", "ds name", ds.Name, "image", image)
	return false, fmt.Sprintf("DaemonSet %s, image %s does not contain DTK data on any layer", ds.Name, image), nil
}

func (p *preflight) PrepareRuntimeInfo(ctx context.Context, image string) (*runtime.RuntimeInformation, error) {
	var err error
	runInfo := p.runtimeAPI.InitRuntimeInfo()
	runInfo.OperatingSystemMajor, runInfo.OperatingSystemMajorMinor, runInfo.OperatingSystemDecimal, err = p.getOSData(ctx, image)
	if err != nil {
		return nil, fmt.Errorf("failed to get OS data in PrepareRuntimeInfo: %w", err)
	}
	runInfo.KernelFullVersion, err = p.getKernelFullVersion(ctx, image)
	if err != nil {
		return nil, fmt.Errorf("failed to get kernel full version in PrepareRuntimeInfo: %w", err)
	}
	runInfo.KernelPatchVersion, err = p.kernelAPI.PatchVersion(runInfo.KernelFullVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to get kernel patch version from kernel version %s in PrepareRuntimeInfo: %w", runInfo.KernelFullVersion, err)
	}

	runInfo.ClusterVersion, runInfo.ClusterVersionMajorMinor, err = p.clusterAPI.Version(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster version info in PrepareRuntimeInfo: %w", err)
	}

	runInfo.OSImageURL, err = p.clusterAPI.OSImageURL(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get os image url in PrepareRuntimeInfo: %w", err)
	}

	return runInfo, nil
}

func (p *preflight) getOSData(ctx context.Context, image string) (string, string, string, error) {
	layer, err := p.registryAPI.LastLayer(ctx, image)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to get last layer of image %s in getOSData: %w", image, err)
	}

	machineOSConfig, err := p.registryAPI.ReleaseImageMachineOSConfig(layer)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to get machine config from image %s in getOSData: %w", image, err)
	}
	return utils.ParseOSInfo(machineOSConfig)
}

func (p *preflight) getKernelFullVersion(ctx context.Context, image string) (string, error) {
	layer, err := p.registryAPI.LastLayer(ctx, image)
	if err != nil {
		return "", fmt.Errorf("failed to get last layer of image %s in getKernelFullVersion: %w", image, err)
	}
	dtkImageURL, err := p.registryAPI.ReleaseManifests(layer)
	if err != nil {
		return "", fmt.Errorf("failed to get driver toolkit image ref from image %s in getKernelFullVersion: %w", image, err)
	}
	if dtkImageURL == "" {
		return "", fmt.Errorf("failed to find the DTK image data in the release image %s in getKernelFullVersion", image)
	}
	dtk, err := p.clusterInfoAPI.GetDTKData(ctx, dtkImageURL)
	if err != nil {
		return "", fmt.Errorf("failed to get DTK data from  image url %s in getKernelFullVersion: %w", dtkImageURL, err)
	}

	return dtk.KernelFullVersion, err
}
