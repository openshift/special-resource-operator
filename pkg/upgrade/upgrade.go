package upgrade

import (
	"runtime"
	"strings"

	"github.com/pkg/errors"

	"github.com/go-logr/logr"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/openshift-psap/special-resource-operator/pkg/cache"
	"github.com/openshift-psap/special-resource-operator/pkg/cluster"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"github.com/openshift-psap/special-resource-operator/pkg/exit"
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
	OSVersion      string
	ClusterVersion string
	DriverToolkit  registry.DriverToolkitEntry
}

func ClusterInfo() (map[string]NodeVersion, error) {

	info, err := NodeVersionInfo()
	exit.OnError(errors.Wrap(err, "Failed to get upgrade info"))

	history, err := cluster.VersionHistory()
	exit.OnError(errors.Wrap(err, "Could not get version history"))

	versions, err := DriverToolkitVersion(history, info)
	exit.OnError(err)

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
			return nil, errors.New("Label " + short + " not found is NFD running? Check node labels")
		}

		short = "feature.node.kubernetes.io/system-os_release.RHEL_VERSION"
		if rhelVersion, found = labels[short]; !found {
			return nil, errors.New("Label " + short + " not found is NFD running? Check node labels")
		}

		short = "feature.node.kubernetes.io/system-os_release.VERSION_ID"
		if clusterVersion, found = labels[short]; !found {
			return nil, errors.New("Label " + short + " not found is NFD running? Check node labels")
		}

		info[kernelFullVersion] = NodeVersion{OSVersion: rhelVersion, ClusterVersion: clusterVersion}
	}

	return info, nil
}

func UpdateInfo(info map[string]NodeVersion, dtk registry.DriverToolkitEntry, imageURL string) (map[string]NodeVersion, error) {
	// Assumes all nodes have the same architecture
	runningArch := runtime.GOARCH
	if runningArch == "amd64" {
		runningArch = "x86_64"
	}
	if !strings.Contains(dtk.KernelFullVersion, runningArch) {
		log.Info("Appending architecture to dtk.KernelFullVersion")
		dtk.KernelFullVersion = dtk.KernelFullVersion + "." + runningArch
		dtk.RTKernelFullVersion = dtk.RTKernelFullVersion + "." + runningArch
		log.Info("Updating version:", "dtk.KernelFullVersion", dtk.KernelFullVersion)
	}

	// First check for the general kernel entry
	if _, ok := info[dtk.KernelFullVersion]; ok {

		dtk.ImageURL = imageURL
		osNFD := info[dtk.KernelFullVersion].OSVersion
		osDTK := dtk.OSVersion

		if osNFD != osDTK {

			msg := "OSVersion mismatch NFD: " + osNFD + " vs. DTK: " + osDTK
			exit.OnError(errors.New(msg))
		}

		nodeVersion := info[dtk.KernelFullVersion]
		nodeVersion.OSVersion = dtk.OSVersion
		nodeVersion.DriverToolkit = dtk

		info[dtk.KernelFullVersion] = nodeVersion

	}

	if _, ok := info[dtk.RTKernelFullVersion]; ok {

		dtk.ImageURL = imageURL
		osNFD := info[dtk.RTKernelFullVersion].OSVersion
		osDTK := dtk.OSVersion

		if osNFD != osDTK {

			msg := "OSVersion mismatch NFD: " + osNFD + " vs. DTK: " + osDTK
			exit.OnError(errors.New(msg))
		}

		nodeVersion := info[dtk.RTKernelFullVersion]
		nodeVersion.OSVersion = dtk.OSVersion
		nodeVersion.DriverToolkit = dtk

		info[dtk.KernelFullVersion] = nodeVersion

	}
	return info, nil
}

func DriverToolkitVersion(entries []string, info map[string]NodeVersion) (map[string]NodeVersion, error) {

	for _, entry := range entries {

		log.Info("History", "entry", entry)
		var layer v1.Layer
		if layer = registry.LastLayer(entry); layer == nil {
			continue
		}
		// For each entry we're fetching the cluster version and dtk URL
		version, imageURL := registry.ReleaseManifests(layer)
		if version == "" {
			exit.OnError(errors.New("Could not extract version from payload"))
		}

		if imageURL == "" {
			warn.OnError(errors.New("No DTK image found, DTK cannot be used in a Build"))
			return info, nil
		}

		if layer = registry.LastLayer(imageURL); layer == nil {
			exit.OnError(errors.New("Cannot extract last layer for DTK from: " + imageURL))
		}

		dtk, err := registry.ExtractToolkitRelease(layer)
		exit.OnError(err)

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
