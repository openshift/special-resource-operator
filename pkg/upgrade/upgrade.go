package upgrade

import (
	"github.com/pkg/errors"

	"github.com/go-logr/logr"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/openshift-psap/special-resource-operator/pkg/cache"
	"github.com/openshift-psap/special-resource-operator/pkg/cluster"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"github.com/openshift-psap/special-resource-operator/pkg/exit"
	"github.com/openshift-psap/special-resource-operator/pkg/registry"
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

		if version != "" {
			log.Info("DTK", "version+imageURL", version+" : "+imageURL)
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
