package upgrade

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	"github.com/openshift-psap/special-resource-operator/pkg/cluster"
	"github.com/openshift-psap/special-resource-operator/pkg/registry"
	"github.com/openshift-psap/special-resource-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	labelKernelVersionFull  = "feature.node.kubernetes.io/kernel-version.full"
	labelOSReleaseVersionID = "feature.node.kubernetes.io/system-os_release.VERSION_ID"

	labelOSReleaseID             = "feature.node.kubernetes.io/system-os_release.ID"
	labelOSReleaseVersionIDMajor = "feature.node.kubernetes.io/system-os_release.VERSION_ID.major"
	labelOSReleaseVersionIDMinor = "feature.node.kubernetes.io/system-os_release.VERSION_ID.minor"
)

type NodeVersion struct {
	OSVersion      string `json:"OSVersion"`
	OSMajor        string `json:"OSMajor"`
	OSMajorMinor   string `json:"OSMajorMinor"`
	ClusterVersion string `json:"clusterVersion"`
}

//go:generate mockgen -source=upgrade.go -package=upgrade -destination=mock_upgrade_api.go

type ClusterInfo interface {
	GetClusterInfo(context.Context, *corev1.NodeList) (map[string]NodeVersion, error)
}

func NewClusterInfo(registry registry.Registry, cluster cluster.Cluster) ClusterInfo {
	return &clusterInfo{
		log:      zap.New(zap.UseDevMode(true)).WithName(utils.Print("upgrade", utils.Blue)),
		registry: registry,
		cluster:  cluster,
	}
}

type clusterInfo struct {
	log      logr.Logger
	registry registry.Registry
	cluster  cluster.Cluster
}

// GetClusterInfo returns a map[full kernel version]NodeVersion
func (ci *clusterInfo) GetClusterInfo(ctx context.Context, nodeList *corev1.NodeList) (map[string]NodeVersion, error) {

	info, err := ci.nodeVersionInfo(nodeList)
	if err != nil {
		return nil, fmt.Errorf("failed to get node info: %w", err)
	}

	return info, nil
}

func (ci *clusterInfo) nodeVersionInfo(nodeList *corev1.NodeList) (map[string]NodeVersion, error) {

	var found bool
	var info = make(map[string]NodeVersion)

	// Assuming all nodes are running the same kernel version,
	// one could easily add driver-kernel-versions for each node.
	for _, node := range nodeList.Items {

		var kernelFullVersion string
		var clusterVersion string

		labels := node.GetLabels()
		// We only need to check for the key, the value
		// is available if the key is there
		if kernelFullVersion, found = labels[labelKernelVersionFull]; !found {
			return nil, fmt.Errorf("label %s not found is NFD running? Check node labels", labelKernelVersionFull)
		}

		if clusterVersion, found = labels[labelOSReleaseVersionID]; !found {
			return nil, fmt.Errorf("label %s not found is NFD running? Check node labels", labelOSReleaseVersionID)
		}

		nodeOSrel := labels[labelOSReleaseID]
		nodeOSmaj := labels[labelOSReleaseVersionIDMajor]
		nodeOSmin := labels[labelOSReleaseVersionIDMinor]
		info[kernelFullVersion] = NodeVersion{OSVersion: nodeOSmaj + "." + nodeOSmin, OSMajor: nodeOSrel + nodeOSmaj, OSMajorMinor: nodeOSrel + nodeOSmaj + "." + nodeOSmin, ClusterVersion: clusterVersion}
	}

	return info, nil
}
