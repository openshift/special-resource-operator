package specialresource

import (
	"strings"

	errs "github.com/pkg/errors"
)

type resourceGroupName struct {
	DriverBuild            string
	DriverContainer        string
	RuntimeEnablement      string
	DevicePlugin           string
	DeviceMonitoring       string
	DeviceGrafana          string
	DeviceFeatureDiscovery string
}

type resourceStateName struct {
	DriverContainer   string
	RuntimeEnablement string
	DevicePlugin      string
	DeviceMonitoring  string
	/*
		"driver-container":   {"specialresource.openshift.io/driver-container-" + hw: "ready"},
		"runtime-enablement": {"specialresource.openshift.io/runtime-enablement-" + hw: "ready"},
		"device-plugin":      {"specialresource.openshift.io/device-plugin-" + hw: "ready"},
		"device-monitoring":  {"specialresource.openshift.io/device-monitoring-" + hw: "ready"},
	*/
}

type runtimeInformation struct {
	OperatingSystem   string
	KernelVersion     string
	ClusterVersion    string
	UpdateVendor      string
	NodeFeature       string
	HardwareResource  string
	OperatorNamespace string

	GroupName resourceGroupName
	StateName resourceStateName
}

var runInfo = runtimeInformation{
	GroupName: resourceGroupName{
		DriverBuild:            "driver-build",
		DriverContainer:        "driver-container",
		RuntimeEnablement:      "runtime-enablement",
		DevicePlugin:           "device-plugin",
		DeviceMonitoring:       "device-monitoring",
		DeviceGrafana:          "device-grafana",
		DeviceFeatureDiscovery: "device-feature-discovery",
	},
	StateName: resourceStateName{
		DriverContainer:   "specialresource.openshift.io/driver-container",
		RuntimeEnablement: "specialresource.openshift.io/runtime-enablement",
		DevicePlugin:      "specialresource.openshift.io/device-plugin",
		DeviceMonitoring:  "specialresource.openshift.io/device-monitoring",
	},
}

func logRuntimeInformation() {
	log.Info("Runtime Information", "OperatorNamespace", runInfo.OperatorNamespace)
	log.Info("Runtime Information", "OperatingSystem", runInfo.OperatingSystem)
	log.Info("Runtime Information", "KernelVersion", runInfo.KernelVersion)
	log.Info("Runtime Information", "ClusterVersion", runInfo.ClusterVersion)
	log.Info("Runtime Information", "UpdateVendor", runInfo.UpdateVendor)
	log.Info("Runtime Information", "NodeFeature", runInfo.NodeFeature)
}

func getRuntimeInformation(r *ReconcileSpecialResource) {

	var err error
	runInfo.OperatingSystem, err = getOperatingSystem()
	exitOnError(errs.Wrap(err, "Failed to get operating system"))

	runInfo.KernelVersion, err = getKernelVersion()
	exitOnError(errs.Wrap(err, "Failed to get kernel version"))

	runInfo.ClusterVersion, err = getClusterVersion()
	exitOnError(errs.Wrap(err, "Failed to get cluster version"))

	runInfo.OperatorNamespace = r.specialresource.GetNamespace()
}

func getOperatingSystem() (string, error) {

	var nodeOSrel string
	var nodeOSver string

	// Assuming all nodes are running the same os
	for _, node := range node.list.Items {
		labels := node.GetLabels()
		nodeOSrel = labels["feature.node.kubernetes.io/system-os_release.ID"]
		nodeOSver = labels["feature.node.kubernetes.io/system-os_release.VERSION_ID.major"]

		if len(nodeOSrel) == 0 || len(nodeOSver) == 0 {
			return "", errs.New("Cannot extract feature.node.kubernetes.io/system-os_release.*, is NFD running? Check node labels")
		}
		break
	}

	return renderOperatingSystem(nodeOSrel, nodeOSver), nil
}

func renderOperatingSystem(rel string, ver string) string {

	log.Info("OS", "rel", rel)
	log.Info("OS", "ver", ver)

	var nodeOS string

	if strings.Compare(rel, "rhcos") == 0 && strings.Compare(ver, "4") == 0 {
		log.Info("Setting OS to rhel8")
		nodeOS = "rhel8"
	}

	if strings.Compare(rel, "rhel") == 0 && strings.Compare(ver, "8") == 0 {
		log.Info("Setting OS to rhel8")
		nodeOS = "rhel8"
	}

	if strings.Compare(rel, "rhel") == 0 && strings.Compare(ver, "7") == 0 {
		log.Info("Setting OS to rhel7")
		nodeOS = "rhel7"
	}

	return nodeOS
}

func getKernelVersion() (string, error) {

	var found bool
	var kernelVersion string
	// Assuming all nodes are running the same kernel version,
	// one could easily add driver-kernel-versions for each node.
	for _, node := range node.list.Items {
		labels := node.GetLabels()

		// We only need to check for the key, the value
		// is available if the key is there
		short := "feature.node.kubernetes.io/kernel-version.full"
		if kernelVersion, found = labels[short]; !found {
			return "", errs.New("Label " + short + " not found is NFD running? Check node labels")
		}
		break
	}

	return kernelVersion, nil
}

func getClusterVersion() (string, error) {
	return "", nil
}
