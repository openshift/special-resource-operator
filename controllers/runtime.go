package controllers

import (
	"context"
	"strconv"
	"strings"

	srov1beta1 "github.com/openshift-psap/special-resource-operator/api/v1beta1"
	"github.com/pkg/errors"
	errs "github.com/pkg/errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	//machineV1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
)

type resourceGroupName struct {
	DriverBuild            string
	DriverContainer        string
	RuntimeEnablement      string
	DevicePlugin           string
	DeviceMonitoring       string
	DeviceGrafana          string
	DeviceFeatureDiscovery string
	CSIDriver              string
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

type proxyConfiguration struct {
	HttpProxy  string
	HttpsProxy string
	NoProxy    string
	TrustedCA  string
}

type runtimeInformation struct {
	OperatingSystemMajor      string
	OperatingSystemMajorMinor string
	OperatingSystemDecimal    string
	KernelVersion             string
	ClusterVersion            string
	ClusterVersionMajorMinor  string
	UpdateVendor              string
	PushSecretName            string
	OSImageURL                string

	Proxy           proxyConfiguration
	GroupName       resourceGroupName
	StateName       resourceStateName
	SpecialResource srov1beta1.SpecialResource
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
		CSIDriver:              "csi-driver",
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
	log.Info("Runtime Information", "OperatingSystemDecimal", runInfo.OperatingSystemDecimal)
	log.Info("Runtime Information", "KernelVersion", runInfo.KernelVersion)
	log.Info("Runtime Information", "ClusterVersion", runInfo.ClusterVersion)
	log.Info("Runtime Information", "ClusterVersionMajorMinor", runInfo.ClusterVersionMajorMinor)
	log.Info("Runtime Information", "UpdateVendor", runInfo.UpdateVendor)
	log.Info("Runtime Information", "PushSecretName", runInfo.PushSecretName)
	log.Info("Runtime Information", "OSImageURL", runInfo.OSImageURL)
	log.Info("Runtime Information", "Proxy", runInfo.Proxy)
}

func getRuntimeInformation(r *SpecialResourceReconciler) {

	var err error
	log.Info("Get Operating System")
	runInfo.OperatingSystemMajor, runInfo.OperatingSystemMajorMinor, runInfo.OperatingSystemDecimal, err = getOperatingSystem()
	exitOnError(errs.Wrap(err, "Failed to get operating system"))

	log.Info("Get Kernel Version")
	runInfo.KernelVersion, err = getKernelVersion()
	exitOnError(errs.Wrap(err, "Failed to get kernel version"))

	log.Info("Get Cluster Version")
	runInfo.ClusterVersion, runInfo.ClusterVersionMajorMinor, err = getClusterVersion()
	exitOnError(errs.Wrap(err, "Failed to get cluster version"))

	log.Info("Get Push Secret Name")
	runInfo.PushSecretName, err = getPushSecretName(r)
	exitOnError(errs.Wrap(err, "Failed to get push secret name"))

	log.Info("Get OS Image URL")
	runInfo.OSImageURL, err = getOSImageURL(r)
	exitOnError(errs.Wrap(err, "Failed to get OSImageURL"))

	log.Info("Get Proxy Configuration")
	runInfo.Proxy, err = getProxyConfiguration(r)
	exitOnError(errs.Wrap(err, "Failed to get Proxy Configuration"))

	r.specialresource.DeepCopyInto(&runInfo.SpecialResource)
}

func getOperatingSystem() (string, string, string, error) {

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
			return "", "", "", errs.New("Cannot extract " + os + ".*, is NFD running? Check node labels")
		}
		break
	}

	return renderOperatingSystem(nodeOSrel, nodeOSmaj, nodeOSmin)
}

func renderOperatingSystem(rel string, maj string, min string) (string, string, string, error) {

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
			return rel + maj, rel + maj + ".0", maj + ".0", nil
		}

		if strings.Compare(maj, "4") == 0 && strings.Compare(min, "4") == 0 {
			maj := "8"
			return rel + maj, rel + maj + ".1", maj + ".1", nil
		}

		if strings.Compare(maj, "4") == 0 && strings.Compare(min, "5") == 0 {
			maj := "8"
			return rel + maj, rel + maj + ".2", maj + ".2", nil
		}

		maj := "8"
		return rel + maj, rel + maj + ".2", maj + ".2", nil
	}

	// A Fedora system has no min yet, so if min is empty
	// return fedora31 and not fedora31.
	if min == "" {
		return rel + maj, rel + maj, maj, nil
	}

	return rel + maj, rel + maj + "." + min, maj + "." + min, nil

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

func getClusterVersion() (string, string, error) {

	version, err := configclient.ClusterVersions().Get(context.TODO(), "version", metav1.GetOptions{})
	if err != nil {
		return "", "", errs.Wrap(err, "ConfigClient unable to get ClusterVersions")
	}

	var majorMinor string
	for _, condition := range version.Status.History {
		if condition.State != "Completed" {
			continue
		}

		s := strings.Split(condition.Version, ".")

		if len(s) > 1 {
			majorMinor = s[0] + "." + s[1]
		} else {
			majorMinor = s[0]
		}

		return condition.Version, majorMinor, nil
	}

	return "", "", errs.New("Undefined Cluster Version")
}

func getPushSecretName(r *SpecialResourceReconciler) (string, error) {

	secrets := &unstructured.UnstructuredList{}

	secrets.SetAPIVersion("v1")
	secrets.SetKind("SecretList")

	log.Info("Getting SecretList")
	opts := []client.ListOption{
		client.InNamespace(r.specialresource.Spec.Namespace),
	}
	err := r.List(context.TODO(), secrets, opts...)
	if err != nil {
		return "", errors.Wrap(err, "Client cannot get SecretList")
	}

	log.Info("Searching for builder-dockercfg Secret")
	for _, secret := range secrets.Items {
		secretName := secret.GetName()

		if strings.Contains(secretName, "builder-dockercfg") {
			log.Info("Found", "Secret", secretName)
			return secretName, nil
		}
	}

	return "", errors.New("Cannot find Secret builder-dockercfg")
}

func getOSImageURL(r *SpecialResourceReconciler) (string, error) {

	cm := &unstructured.Unstructured{}
	cm.SetAPIVersion("v1")
	cm.SetKind("ConfigMap")

	namespacedName := types.NamespacedName{Namespace: "openshift-machine-config-operator", Name: "machine-config-osimageurl"}
	err := r.Get(context.TODO(), namespacedName, cm)
	if apierrors.IsNotFound(err) {
		return "", errs.Wrap(err, "ConfigMap machine-config-osimageurl -n  openshift-machine-config-operator not found")
	}

	osImageURL, found, err := unstructured.NestedString(cm.Object, "data", "osImageURL")
	exitOnErrorOrNotFound(found, err)

	return osImageURL, nil

}

func getProxyConfiguration(r *SpecialResourceReconciler) (proxyConfiguration, error) {

	proxy := proxyConfiguration{}

	cfgs := &unstructured.UnstructuredList{}
	cfgs.SetAPIVersion("config.openshift.io/v1")
	cfgs.SetKind("ProxyList")

	opts := []client.ListOption{}

	err := r.List(context.TODO(), cfgs, opts...)
	if err != nil {
		return proxy, errors.Wrap(err, "Client cannot get ProxyList")
	}

	for _, cfg := range cfgs.Items {
		cfgName := cfg.GetName()

		var fnd bool
		var err error
		// If no proxy is configured, we do not exit we just give a warning
		// and initialized the Proxy struct with zero sized strings
		if strings.Contains(cfgName, "cluster") {
			if proxy.HttpProxy, fnd, err = unstructured.NestedString(cfg.Object, "spec", "httpProxy"); err != nil {
				warnOnErrorOrNotFound(fnd, err)
				proxy.HttpProxy = ""
			}

			if proxy.HttpsProxy, fnd, err = unstructured.NestedString(cfg.Object, "spec", "httpsProxy"); err != nil {
				warnOnErrorOrNotFound(fnd, err)
				proxy.HttpsProxy = ""
			}

			if proxy.NoProxy, fnd, err = unstructured.NestedString(cfg.Object, "spec", "noProxy"); err != nil {
				warnOnErrorOrNotFound(fnd, err)
				proxy.NoProxy = ""
			}

			if proxy.TrustedCA, fnd, err = unstructured.NestedString(cfg.Object, "spec", "trustedCA", "name"); err != nil {
				warnOnErrorOrNotFound(fnd, err)
				proxy.TrustedCA = ""
			}
		}
	}

	return proxy, nil
}

func setupProxy(obj *unstructured.Unstructured, r *SpecialResourceReconciler) error {

	if strings.Compare(obj.GetKind(), "Pod") == 0 {
		if err := setupPodProxy(obj, r); err != nil {
			return errs.Wrap(err, "Cannot setup Pod Proxy")
		}
	}
	if strings.Compare(obj.GetKind(), "DaemonSet") == 0 {
		if err := setupDaemonSetProxy(obj, r); err != nil {
			return errs.Wrap(err, "Cannot setup DaemonSet Proxy")
		}

	}

	return nil
}

// We may generalize more depending on how many entities need proxy settings.
// path... -> Pod, DaemonSet, BuildConfig, etc.
func setupDaemonSetProxy(obj *unstructured.Unstructured, r *SpecialResourceReconciler) error {
	containers, found, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	exitOnErrorOrNotFound(found, err)

	if err := setupContainersProxy(containers); err != nil {
		return errs.Wrap(err, "Cannot set proxy for Pod")
	}

	return nil
}

func setupPodProxy(obj *unstructured.Unstructured, r *SpecialResourceReconciler) error {

	containers, found, err := unstructured.NestedSlice(obj.Object, "spec", "containers")
	exitOnErrorOrNotFound(found, err)

	if err := setupContainersProxy(containers); err != nil {
		return errs.Wrap(err, "Cannot set proxy for Pod")
	}

	return nil
}

func setupContainersProxy(containers []interface{}) error {

	for _, container := range containers {
		switch container := container.(type) {
		case map[string]interface{}:
			env, found, err := unstructured.NestedSlice(container, "env")
			exitOnError(err)

			// If env not found we are creating a new env slice
			// otherwise we're appending it to the existing env slice
			httpproxy := make(map[string]interface{})
			httpsproxy := make(map[string]interface{})
			noproxy := make(map[string]interface{})

			httpproxy["name"] = "HTTP_PROXY"
			httpproxy["value"] = runInfo.Proxy.HttpProxy

			httpsproxy["name"] = "HTTPS_PROXY"
			httpsproxy["value"] = runInfo.Proxy.HttpsProxy

			noproxy["name"] = "NO_PROXY"
			noproxy["value"] = runInfo.Proxy.NoProxy

			if !found {
				env = make([]interface{}, 0)
			}

			env = append(env, httpproxy)
			env = append(env, httpsproxy)
			env = append(env, noproxy)

			if err := unstructured.SetNestedSlice(container, env, "env"); err != nil {
				errs.Wrap(err, "Cannot set env for container")
			}

		default:
			log.Info("container", "DEFAULT NOT THE CORRECT TYPE", container)
		}
		break
	}

	return nil
}
