package upgrade

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/pkg/errors"

	"github.com/go-logr/logr"
	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/openshift-psap/special-resource-operator/pkg/cache"
	"github.com/openshift-psap/special-resource-operator/pkg/cluster"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"github.com/openshift-psap/special-resource-operator/pkg/registry"
	"github.com/openshift-psap/special-resource-operator/pkg/warn"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	log logr.Logger
)

func init() {
	log = zap.New(zap.UseDevMode(true)).WithName(color.Print("upgrade", color.Blue))
}

type NodeVersion struct {
	OSVersion      string                      `json:"OSVersion"`
	OSMajor        string                      `json:"OSMajor"`
	OSMajorMinor   string                      `json:"OSMajorMinor"`
	ClusterVersion string                      `json:"clusterVersion"`
	DriverToolkit  registry.DriverToolkitEntry `json:"driverToolkit"`
}

func ClusterInfo() (map[string]NodeVersion, error) {

	info, err := NodeVersionInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get upgrade info: %w", err)
	}

	history, err := cluster.VersionHistory()
	if err != nil {
		return nil, fmt.Errorf("could not get version history: %w", err)
	}

	versions, err := DriverToolkitVersion(history, info)
	if err != nil {
		return nil, err
	}

	return versions, nil

}

func NodeVersionInfo() (map[string]NodeVersion, error) {

	var found bool
	var info = make(map[string]NodeVersion)

	// Assuming all nodes are running the same kernel version,
	// one could easily add driver-kernel-versions for each node.
	for _, node := range cache.Node.List.Items {

		var rhelVersion string
		var kernelFullVersion string
		var clusterVersion string

		labels := node.GetLabels()
		// We only need to check for the key, the value
		// is available if the key is there
		short := "feature.node.kubernetes.io/kernel-version.full"
		if kernelFullVersion, found = labels[short]; !found {
			return nil, fmt.Errorf("label %s not found is NFD running? Check node labels", short)
		}

		short = "feature.node.kubernetes.io/system-os_release.VERSION_ID"
		if clusterVersion, found = labels[short]; !found {
			return nil, fmt.Errorf("label %s not found is NFD running? Check node labels", short)
		}

		short = "feature.node.kubernetes.io/system-os_release.RHEL_VERSION"
		if rhelVersion, found = labels[short]; !found {
			nodeOSrel := labels["feature.node.kubernetes.io/system-os_release.ID"]
			nodeOSmaj := labels["feature.node.kubernetes.io/system-os_release.VERSION_ID.major"]
			nodeOSmin := labels["feature.node.kubernetes.io/system-os_release.VERSION_ID.minor"]
			info[kernelFullVersion] = NodeVersion{OSVersion: nodeOSmaj + "." + nodeOSmin, OSMajor: nodeOSrel + nodeOSmaj, OSMajorMinor: nodeOSrel + nodeOSmaj + "." + nodeOSmin, ClusterVersion: clusterVersion}
		} else {
			rhelMaj := rhelVersion[0:1]
			info[kernelFullVersion] = NodeVersion{OSVersion: rhelVersion, OSMajor: "rhel" + rhelMaj, OSMajorMinor: "rhel" + rhelVersion, ClusterVersion: clusterVersion}
		}
	}

	return info, nil
}

func UpdateInfo(info map[string]NodeVersion, dtk registry.DriverToolkitEntry, imageURL string) (map[string]NodeVersion, error) {
	dtk.ImageURL = imageURL
	osDTK := dtk.OSVersion
	// Assumes all nodes have the same architecture
	runningArch := runtime.GOARCH
	switch runningArch {
	case "amd64":
		runningArch = "x86_64"
	case "arm64":
		runningArch = "aarch64"
	}
	if !strings.Contains(dtk.KernelFullVersion, runningArch) {
		dtk.KernelFullVersion = dtk.KernelFullVersion + "." + runningArch
		dtk.RTKernelFullVersion = dtk.RTKernelFullVersion + "." + runningArch
		log.Info("Updating version:", "dtk.KernelFullVersion", dtk.KernelFullVersion, "dtk.RTKernelFullVersion", dtk.RTKernelFullVersion)
	}

	if _, ok := info[dtk.KernelFullVersion]; ok {
		osNFD := info[dtk.KernelFullVersion].OSVersion

		if osNFD != osDTK {
			return nil, fmt.Errorf("OSVersion mismatch NFD: %s vs. DTK: %s", osNFD, osDTK)
		}

		nodeVersion := info[dtk.KernelFullVersion]
		nodeVersion.OSVersion = dtk.OSVersion
		nodeVersion.DriverToolkit = dtk

		info[dtk.KernelFullVersion] = nodeVersion

	}

	if _, ok := info[dtk.RTKernelFullVersion]; ok {
		osNFD := info[dtk.RTKernelFullVersion].OSVersion

		if osNFD != osDTK {
			return nil, fmt.Errorf("OSVersion mismatch NFD: %s vs. DTK: %s", osNFD, osDTK)
		}

		nodeVersion := info[dtk.RTKernelFullVersion]
		nodeVersion.OSVersion = dtk.OSVersion
		nodeVersion.DriverToolkit = dtk

		info[dtk.RTKernelFullVersion] = nodeVersion

	}
	return info, nil
}

func DriverToolkitVersion(entries []string, info map[string]NodeVersion) (map[string]NodeVersion, error) {

	for _, entry := range entries {

		log.Info("History", "entry", entry)

		var (
			err   error
			layer v1.Layer
		)

		layer, err = registry.LastLayer(entry)
		if err != nil {
			return nil, err
		}

		if layer == nil {
			continue
		}
		// For each entry we're fetching the cluster version and dtk URL
		_, imageURL, err := registry.ReleaseManifests(layer)
		if err != nil {
			return nil, fmt.Errorf("could not extract version from payload: %w", err)
		}

		if imageURL == "" {
			warn.OnError(errors.New("No DTK image found, DTK cannot be used in a Build"))
			return info, nil
		}

		if layer, err = registry.LastLayer(imageURL); layer == nil {
			return nil, fmt.Errorf("cannot extract last layer for DTK from %s: %w", imageURL, err)
		}

		dtk, err := registry.ExtractToolkitRelease(layer)
		if err != nil {
			return nil, err
		}

		// info has the kernels that are currently "running" on the cluster
		// we're going only to update the struct with DTK information on
		// running kernels and not on all that are found.
		// We could have many entries with DTKs that are from an old update
		// The objects that are kernel affine should only be replicated
		// for valid kernels.
		return UpdateInfo(info, dtk, imageURL)

	}

	return info, nil
}
