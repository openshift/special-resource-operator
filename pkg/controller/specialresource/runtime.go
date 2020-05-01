package specialresource

import (
	"strconv"
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
	OperatingSystemMajor      string
	OperatingSystemMajorMinor string
	KernelVersion             string
	ClusterVersion            string
	UpdateVendor              string
	NodeFeature               string
	HardwareResource          string
	OperatorNamespace         string

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
	log.Info("Runtime Information", "OperatingSystemMajor", runInfo.OperatingSystemMajor)
	log.Info("Runtime Information", "OperatingSystemMajorMinor", runInfo.OperatingSystemMajorMinor)
	log.Info("Runtime Information", "KernelVersion", runInfo.KernelVersion)
	log.Info("Runtime Information", "ClusterVersion", runInfo.ClusterVersion)
	log.Info("Runtime Information", "UpdateVendor", runInfo.UpdateVendor)
	log.Info("Runtime Information", "NodeFeature", runInfo.NodeFeature)
}

func getRuntimeInformation(r *ReconcileSpecialResource) {

	var err error
	runInfo.OperatingSystemMajor, runInfo.OperatingSystemMajorMinor, err = getOperatingSystem()
	exitOnError(errs.Wrap(err, "Failed to get operating system"))

	runInfo.KernelVersion, err = getKernelVersion()
	exitOnError(errs.Wrap(err, "Failed to get kernel version"))

	runInfo.ClusterVersion, err = getClusterVersion()
	exitOnError(errs.Wrap(err, "Failed to get cluster version"))

	runInfo.OperatorNamespace = r.specialresource.GetNamespace()
}

func getOperatingSystem() (string, string, error) {

	var nodeOSrel string
	var nodeOSmaj string
	var nodeOSmin string

	// Assuming all nodes are running the same os

	os := "feature.node.kubernetes.io/system-os_release"

	for _, node := range node.list.Items {
		labels := node.GetLabels()
		nodeOSrel = labels[os+".ID"]
		nodeOSmaj = labels[os+".VERSION_ID.major"]
		nodeOSmin = labels[os+".VERSION_ID.minor"]

		if len(nodeOSrel) == 0 || len(nodeOSmaj) == 0 {
			return "", "", errs.New("Cannot extract " + os + ".*, is NFD running? Check node labels")
		}
		break
	}

	return renderOperatingSystem(nodeOSrel, nodeOSmaj, nodeOSmin)
}

func renderOperatingSystem(rel string, maj string, min string) (string, string, error) {

	log.Info("OS", "rel", rel)
	log.Info("OS", "maj", maj)
	log.Info("OS", "min", min) // this can be empty e.g fedora30

	// rhcos version is the openshift version running need to translate
	// into rhel major minor version
	if strings.Compare(rel, "rhcos") == 0 {
		rel := "rhel"

		num, _ := strconv.Atoi(min)

		if strings.Compare(maj, "4") == 0 && num < 4 {
			maj := "8"
			return rel + maj, rel + maj + ".0", nil
		}
		maj := "8"
		return rel + maj, rel + maj + ".1", nil
	}

	// A Fedora system has no min yet, so if min is empty
	// return fedora31 and not fedora31.
	if min == "" {
		return rel + maj, rel + maj, nil
	}

	return rel + maj, rel + maj + "." + min, nil

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
