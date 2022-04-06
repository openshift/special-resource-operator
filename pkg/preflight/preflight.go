package preflight

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
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
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	VerificationStatusReasonBuildConfigPresent = "Verification successful, BuildConfig is present in the recipe"
	VerificationStatusReasonNoDaemonSet        = "Verification successful, no driver-container present in the recipe"
	VerificationStatusReasonUnknown            = "Verification has not started yet"
	VerificationStatusReasonVerified           = "Verification successful, driver-container for the next kernel version is present"
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
		log:            zap.New(zap.UseDevMode(true)).WithName(utils.Print("preflight", utils.Blue)),
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
	log            logr.Logger
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

	p.log.Info("Start preflight for cr", "name", sr.Name)
	sr.DeepCopyInto(&runInfo.SpecialResource)

	chart, err := p.helmerAPI.Load(sr.Spec.Chart)
	if err != nil {
		p.log.Error(err, "Failed to load helm chart for CR", "name", sr.Name)
		return false, fmt.Sprintf("Failed to load helm chart for CR %s", sr.Name), err
	}

	yamlsList, err := p.processFullChartTemplates(ctx, chart, sr.Spec.Set, runInfo, sr.Namespace, runInfo.KernelFullVersion)
	if err != nil {
		p.log.Error(err, "Failed to process full chart during preflight check")
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

	p.log.Info("Start preflight check for:", "helm", fullChart.Name())

	nodeVersion := runInfo.ClusterUpgradeInfo[upgradeKernelVersion]

	runInfo.ClusterVersionMajorMinor = nodeVersion.ClusterVersion
	runInfo.OperatingSystemDecimal = nodeVersion.OSVersion
	runInfo.OperatingSystemMajorMinor = nodeVersion.OSMajorMinor
	runInfo.OperatingSystemMajor = nodeVersion.OSMajor
	runInfo.DriverToolkitImage = nodeVersion.DriverToolkit.ImageURL

	fullChart.Values, err = chartutil.CoalesceValues(&fullChart, values.Object)
	if err != nil {
		p.log.Error(err, "failed to coalesce CR values for chart during preflight")
		return "", err
	}
	rinfo, err := k8sruntime.DefaultUnstructuredConverter.ToUnstructured(&runInfo)
	if err != nil {
		p.log.Error(err, "failed to convert runInfo type to unstructured")
		return "", err
	}

	fullChart.Values, err = chartutil.CoalesceValues(&fullChart, rinfo)
	if err != nil {
		p.log.Error(err, "failed to coalesce run info values into chart during preflight")
		return "", err
	}

	return p.helmerAPI.GetHelmOutput(ctx, fullChart, fullChart.Values, namespace)
}

//[TODO] - handle multiple daemonsets and buildconfigs
func (p *preflight) handleYAMLsCheck(ctx context.Context, yamlsList string, upgradeKernelVersion string) (bool, string, error) {
	objList, err := p.resourceAPI.GetObjectsFromYAML([]byte(yamlsList))
	if err != nil {
		p.log.Error(err, "failed to extract object from chart yaml list during preflight")
		return false, "Failed to extract object from chart yaml list during preflight", err
	}

	for _, obj := range objList.Items {
		kind := obj.GetKind()
		if kind == "BuildConfig" {
			// no more need to check daemons set, build config is present
			p.log.Info("preflight: buildconfig related to daemonset, skipping image verification")
			return true, VerificationStatusReasonBuildConfigPresent, nil
		}
		if kind == "DaemonSet" {
			return p.daemonSetPreflightCheck(ctx, &obj, upgradeKernelVersion)
		}
	}
	return true, VerificationStatusReasonNoDaemonSet, nil
}

func (p *preflight) daemonSetPreflightCheck(ctx context.Context, obj *unstructured.Unstructured, upgradeKernelVersion string) (bool, string, error) {
	var ds k8sv1.DaemonSet
	err := k8sruntime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &ds)
	if err != nil {
		p.log.Error(err, "failed to convert YAML daemonset into struct daemonset")
		return false, "failed to convert YAML daemonset into struct daemonset", err
	}
	if len(ds.Spec.Template.Spec.Containers) == 0 {
		p.log.Error(nil, "invalid daemonset, no container  data present")
		return false, "invalid daemonset, no container  data present", fmt.Errorf("invalid daemonset, no container  data present")
	}
	image := ds.Spec.Template.Spec.Containers[0].Image

	p.log.Info("daemonset image for preflight validation", "image", image)

	repo, digests, auth, err := p.registryAPI.GetLayersDigests(ctx, image)
	if err != nil {
		p.log.Info("Failed to get layers digests for image", "image", image)
		return false, fmt.Sprintf("image %s is inaccessible or does not exists", image), nil
	}

	for i := len(digests) - 1; i >= 0; i-- {
		layer, err := p.registryAPI.GetLayerByDigest(repo, digests[i], auth)
		if err != nil {
			p.log.Info("Failed to extract/access layer from image", "layer", digests[i], "image", image)
			return false, fmt.Sprintf("image %s layer %s is inaccessible", image, digests[i]), nil
		}
		dtk, err := p.registryAPI.ExtractToolkitRelease(layer)
		if err != nil {
			p.log.Info("dtk info not present", "image", image, "layerIndex", i)
			continue
		}

		p.log.Info("dtk info present in layer", "layerIndex", i)
		if dtk.KernelFullVersion != upgradeKernelVersion {
			p.log.Info("DTK kernel version differs from the upgrade node version", "dtkVersion", dtk.KernelFullVersion, "upgradeVersion", upgradeKernelVersion)
			return false, fmt.Sprintf("image kernel version %s different from upgrade kernel version %s", dtk.KernelFullVersion, upgradeKernelVersion), nil
		}
		return true, VerificationStatusReasonVerified, nil
	}

	p.log.Info("DTK info not present on any layer of the image, invaid image format", "image", image)
	return false, fmt.Sprintf("image %s does not contain DTK data on any layer", image), nil
}

func (p *preflight) PrepareRuntimeInfo(ctx context.Context, image string) (*runtime.RuntimeInformation, error) {
	var err error
	runInfo := p.runtimeAPI.InitRuntimeInfo()
	runInfo.OperatingSystemMajor, runInfo.OperatingSystemMajorMinor, runInfo.OperatingSystemDecimal, err = p.getOSData(ctx, image)
	if err != nil {
		p.log.Error(err, "Failed to get os data for preflight")
		return nil, err
	}
	runInfo.KernelFullVersion, err = p.getKernelFullVersion(ctx, image)
	if err != nil {
		p.log.Error(err, "Failed to get kernel full version for preflight")
		return nil, err
	}
	runInfo.KernelPatchVersion, err = p.kernelAPI.PatchVersion(runInfo.KernelFullVersion)
	if err != nil {
		p.log.Error(err, "Failed to get kernel patch version for preflight", "fullVersion", runInfo.KernelPatchVersion)
		return nil, err
	}

	runInfo.ClusterVersion, runInfo.ClusterVersionMajorMinor, err = p.clusterAPI.Version(ctx)
	if err != nil {
		p.log.Error(err, "Failed to get cluster version info for preflight")
		return nil, err
	}

	runInfo.OSImageURL, err = p.clusterAPI.OSImageURL(ctx)
	if err != nil {
		p.log.Error(err, "failed to os image url for preflight")
		return nil, err
	}

	return runInfo, nil
}

func (p *preflight) getOSData(ctx context.Context, image string) (string, string, string, error) {
	layer, err := p.registryAPI.LastLayer(ctx, image)
	if err != nil {
		p.log.Error(err, "failed to get last layer of image", "image", image)
		return "", "", "", err
	}

	machineOSConfig, err := p.registryAPI.ReleaseImageMachineOSConfig(layer)
	if err != nil {
		p.log.Error(err, "failed to get machine os config from image", "image", image)
		return "", "", "", err
	}
	return utils.ParseOSInfo(machineOSConfig)
}

func (p *preflight) getKernelFullVersion(ctx context.Context, image string) (string, error) {
	layer, err := p.registryAPI.LastLayer(ctx, image)
	if err != nil {
		p.log.Error(err, "failed to get last layer of image", "image", image)
		return "", err
	}
	dtkImageURL, err := p.registryAPI.ReleaseManifests(layer)
	if err != nil {
		p.log.Error(err, "failed to get driver toolkit image ref from image", "image", image)
		return "", err
	}
	if dtkImageURL == "" {
		p.log.Info("failed to find dtk image in the release manifests")
		return "", fmt.Errorf("Failed to find the DTK image data in the release")
	}
	dtk, err := p.clusterInfoAPI.GetDTKData(ctx, dtkImageURL)
	if err != nil {
		p.log.Error(err, "failed to get dtk data from dtk image", "image", dtkImageURL)
		return "", err
	}

	return dtk.KernelFullVersion, err
}
