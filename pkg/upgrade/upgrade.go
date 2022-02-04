package upgrade

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/go-logr/logr"

	"github.com/openshift-psap/special-resource-operator/pkg/cluster"
	"github.com/openshift-psap/special-resource-operator/pkg/registry"
	"github.com/openshift-psap/special-resource-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type NodeVersion struct {
	OSVersion      string                      `json:"OSVersion"`
	OSMajor        string                      `json:"OSMajor"`
	OSMajorMinor   string                      `json:"OSMajorMinor"`
	ClusterVersion string                      `json:"clusterVersion"`
	DriverToolkit  registry.DriverToolkitEntry `json:"driverToolkit"`
}

//go:generate mockgen -source=upgrade.go -package=upgrade -destination=mock_upgrade_api.go

type ClusterInfo interface {
	GetClusterInfo(context.Context, *corev1.NodeList) (map[string]NodeVersion, error)
}

func NewClusterInfo(reg registry.Registry, cluster cluster.Cluster) ClusterInfo {
	return &clusterInfo{
		log:      zap.New(zap.UseDevMode(true)).WithName(utils.Print("upgrade", utils.Blue)),
		registry: reg,
		cluster:  cluster,
		cache:    make(map[string]*registry.DriverToolkitEntry),
	}
}

type clusterInfo struct {
	log      logr.Logger
	registry registry.Registry
	cluster  cluster.Cluster
	cache    map[string]*registry.DriverToolkitEntry
}

// GetClusterInfo returns a map[full kernel version]NodeVersion
func (ci *clusterInfo) GetClusterInfo(ctx context.Context, nodeList *corev1.NodeList) (map[string]NodeVersion, error) {

	info, err := ci.nodeVersionInfo(nodeList)
	if err != nil {
		return nil, err
	}

	dtkImages, err := ci.cluster.GetDTKImages(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not get DTK images: %w", err)
	}

	versions, err := ci.driverToolkitVersion(ctx, dtkImages, info)
	if err != nil {
		return nil, err
	}

	return versions, nil
}

func (ci *clusterInfo) nodeVersionInfo(nodeList *corev1.NodeList) (map[string]NodeVersion, error) {
	info := make(map[string]NodeVersion)
	// Assuming all nodes are running the same kernel version,
	// one could easily add driver-kernel-versions for each node.
	for _, node := range nodeList.Items {
		kernelFullVersion := node.Status.NodeInfo.KernelVersion
		if len(kernelFullVersion) == 0 {
			return nil, fmt.Errorf("kernel version not found in node %s", node.Name)
		}

		clusterVersion, osVersion, osMajor, err := utils.ParseOSInfo(node.Status.NodeInfo.OSImage)
		if err != nil {
			return nil, err
		}
		info[kernelFullVersion] = NodeVersion{
			OSVersion:      osVersion,
			ClusterVersion: clusterVersion,
			OSMajor:        "rhel" + osMajor,
			OSMajorMinor:   "rhel" + osVersion,
		}
	}

	return info, nil
}

func (ci *clusterInfo) updateInfo(info map[string]NodeVersion, dtk registry.DriverToolkitEntry, imageURL string) (map[string]NodeVersion, error) {
	dtk.ImageURL = imageURL
	osDTK := dtk.OSVersion
	// Assumes all nodes have the same architecture
	runningArch := runtime.GOARCH
	ci.log.Info("Runtime GOARCH is:", "runningArch", runningArch)
	ci.log.Info("dtk.KernelFullVersion is:", "kernelVersion", dtk.KernelFullVersion)
	switch runningArch {
	case "amd64":
		runningArch = "x86_64"
	case "arm64":
		runningArch = "aarch64"
	}
	if !strings.Contains(dtk.KernelFullVersion, runningArch) {
		dtk.KernelFullVersion = dtk.KernelFullVersion + "." + runningArch
		dtk.RTKernelFullVersion = dtk.RTKernelFullVersion + "." + runningArch
		ci.log.Info("Updating version:", "dtk.KernelFullVersion", dtk.KernelFullVersion, "dtk.RTKernelFullVersion", dtk.RTKernelFullVersion)
	}

	match := false

	if nodeVersion, ok := info[dtk.KernelFullVersion]; ok {
		osNode := nodeVersion.OSVersion
		if osNode != osDTK {
			return nil, fmt.Errorf("OSVersion mismatch Node: %s vs. DTK: %s", osNode, osDTK)
		}
		nodeVersion.DriverToolkit = dtk
		info[dtk.KernelFullVersion] = nodeVersion
		match = true
	}

	if nodeVersion, ok := info[dtk.RTKernelFullVersion]; ok {
		osNode := nodeVersion.OSVersion
		if osNode != osDTK {
			return nil, fmt.Errorf("OSVersion mismatch Node: %s vs. DTK: %s", osNode, osDTK)
		}
		nodeVersion.DriverToolkit = dtk
		info[dtk.RTKernelFullVersion] = nodeVersion
		match = true
	}

	if !match {
		return nil, fmt.Errorf("DTK kernel not found running in the cluster. kernelFullVersion: %s. rtKernelFullVersion: %s", dtk.KernelFullVersion, dtk.RTKernelFullVersion)
	}

	return info, nil
}

func (ci *clusterInfo) driverToolkitVersion(ctx context.Context, dtkImages []string, info map[string]NodeVersion) (map[string]NodeVersion, error) {
	if len(dtkImages) == 0 {
		return info, nil
	}

	var dtk *registry.DriverToolkitEntry
	imageURL := dtkImages[0]

	if dtk = ci.cache[imageURL]; dtk != nil {
		ci.log.Info("History from cache", "imageURL", imageURL, "dtk", dtk)
	} else {
		layer, err := ci.registry.LastLayer(ctx, imageURL)
		if err != nil {
			return nil, err
		}
		if layer == nil {
			return nil, fmt.Errorf("cannot extract last layer for DTK from %s: %w", imageURL, err)
		}

		dtk, err = ci.registry.ExtractToolkitRelease(layer)
		if err != nil {
			return nil, err
		}

		ci.cache[imageURL] = dtk
		ci.log.Info("History added to cache", "imageURL", imageURL, "dtk", dtk)
	}
	// info has the kernels that are currently "running" on the cluster
	// we're going only to update the struct with DTK information on
	// running kernels and not on all that are found.
	// We could have many entries with DTKs that are from an old update
	// The objects that are kernel affine should only be replicated
	// for valid kernels.
	return ci.updateInfo(info, *dtk, imageURL)
}
