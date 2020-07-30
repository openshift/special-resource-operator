package specialresource

import (
	"context"
	"strconv"
	"strings"

	srov1alpha1 "github.com/openshift-psap/special-resource-operator/pkg/apis/sro/v1alpha1"
	"github.com/pkg/errors"
	errs "github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	PushSecretName            string

	GroupName       resourceGroupName
	StateName       resourceStateName
	SpecialResource srov1alpha1.SpecialResource
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
	log.Info("Runtime Information", "OperatingSystemMajor", runInfo.OperatingSystemMajor)
	log.Info("Runtime Information", "OperatingSystemMajorMinor", runInfo.OperatingSystemMajorMinor)
	log.Info("Runtime Information", "KernelVersion", runInfo.KernelVersion)
	log.Info("Runtime Information", "ClusterVersion", runInfo.ClusterVersion)
	log.Info("Runtime Information", "UpdateVendor", runInfo.UpdateVendor)
	log.Info("Runtime Information", "PushSecretName", runInfo.PushSecretName)
}

func getRuntimeInformation(r *ReconcileSpecialResource) {

	var err error
	runInfo.OperatingSystemMajor, runInfo.OperatingSystemMajorMinor, err = getOperatingSystem()
	exitOnError(errs.Wrap(err, "Failed to get operating system"))

	runInfo.KernelVersion, err = getKernelVersion()
	exitOnError(errs.Wrap(err, "Failed to get kernel version"))

	runInfo.ClusterVersion, err = getClusterVersion()
	exitOnError(errs.Wrap(err, "Failed to get cluster version"))

	runInfo.PushSecretName, err = getPushSecretName(r)
	exitOnError(errs.Wrap(err, "Failed to get push secret name"))

	r.specialresource.DeepCopyInto(&runInfo.SpecialResource)
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

		log.Info("DEBUG", "LOG", labels[os+".ID"])
		log.Info("DEBUG", "LOG", labels[os+".VERSION_ID.major"])

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

		if strings.Compare(maj, "4") == 0 && strings.Compare(min, "4") == 0 {
			maj := "8"
			return rel + maj, rel + maj + ".1", nil
		}

		if strings.Compare(maj, "4") == 0 && strings.Compare(min, "5") == 0 {
			maj := "8"
			return rel + maj, rel + maj + ".2", nil
		}

		maj := "8"
		return rel + maj, rel + maj + ".2", nil
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

	version, err := configclient.ClusterVersions().Get("version", metav1.GetOptions{})
	if err != nil {
		return "", errs.Wrap(err, "ConfigClient unable to get ClusterVersions")
	}

	for _, condition := range version.Status.History {
		if condition.State != "Completed" {
			continue
		}

		return condition.Version, nil
	}

	return "", errs.New("Undefined Cluster Version")
}

func getPushSecretName(r *ReconcileSpecialResource) (string, error) {

	secrets := &unstructured.UnstructuredList{}

	secrets.SetAPIVersion("v1")
	secrets.SetKind("SecretList")

	opts := &client.ListOptions{}
	opts.InNamespace(r.specialresource.GetNamespace())

	err := r.client.List(context.TODO(), opts, secrets)
	if err != nil {
		return "", errors.Wrap(err, "Client cannot get SecretList")
	}

	log.Info("DEBUG", "SECRET LEN", len(secrets.Items))

	for _, secret := range secrets.Items {
		secretName := secret.GetName()

		log.Info("DEBUG", "SECRET", secretName)

		if strings.Contains(secretName, "builder-dockercfg") {
			return secretName, nil
		}
	}

	return "", errors.Wrap(err, "Cannot find Secret builder-dockercfg")
}
