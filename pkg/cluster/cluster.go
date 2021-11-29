package cluster

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/openshift-psap/special-resource-operator/pkg/cache"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"github.com/openshift-psap/special-resource-operator/pkg/osversion"
	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var log = zap.New(zap.UseDevMode(true)).WithName(color.Print("cache", color.Brown))

func Version() (string, string, error) {

	available, err := ClusterVersionAvailable()
	if err != nil {
		return "", "", err
	}
	if !available {
		return "", "", nil
	}

	version, err := clients.Interface.ClusterVersionGet(context.TODO(), metav1.GetOptions{})
	if err != nil {
		return "", "", fmt.Errorf("ConfigClient unable to get ClusterVersions: %w", err)
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

	return "", "", errors.New("Undefined Cluster Version")
}

func VersionHistory() ([]string, error) {

	stat := []string{}

	available, err := ClusterVersionAvailable()
	if err != nil {
		return nil, err
	}
	if !available {
		return stat, nil
	}

	version, err := clients.Interface.ClusterVersionGet(context.TODO(), metav1.GetOptions{})
	if err != nil {
		return stat, fmt.Errorf("ConfigClient unable to get ClusterVersions: %w", err)
	}

	stat = append(stat, version.Status.Desired.Image)

	for _, condition := range version.Status.History {
		if condition.State == "Completed" {
			stat = append(stat, condition.Image)
		}
	}

	return stat, nil
}

func OSImageURL() (string, error) {

	machineConfigAvailable, err := clients.Interface.HasResource(machinev1.SchemeGroupVersion.WithResource("machineconfigs"))
	if err != nil {
		return "", fmt.Errorf("Error discovering machineconfig API resource: %w", err)
	}
	if !machineConfigAvailable {
		log.Info("Warning: Could not find machineconfig API resource. Can be ignored on vanilla k8s.")
		return "", nil
	}

	cm := &unstructured.Unstructured{}
	cm.SetAPIVersion("v1")
	cm.SetKind("ConfigMap")

	namespacedName := types.NamespacedName{Namespace: "openshift-machine-config-operator", Name: "machine-config-osimageurl"}
	err = clients.Interface.Get(context.TODO(), namespacedName, cm)
	if apierrors.IsNotFound(err) {
		return "", fmt.Errorf("ConfigMap machine-config-osimageurl -n  openshift-machine-config-operator not found: %w", err)
	}

	osImageURL, found, err := unstructured.NestedString(cm.Object, "data", "osImageURL")
	if err != nil {
		return "", err
	}
	if !found {
		return "", errors.New("osImageURL not found")
	}

	return osImageURL, nil
}

// Assumes all nodes have the same OS.
// Returns the os in the following forms:
// rhelx.y, rhelx, x.y
func OperatingSystem() (string, string, string, error) {

	var nodeOSrel string
	var nodeOSmaj string
	var nodeOSmin string
	var labels map[string]string

	// Assuming all nodes are running the same os
	os := "feature.node.kubernetes.io/system-os_release"

	for _, node := range cache.Node.List.Items {
		labels = node.GetLabels()
		nodeOSrel = labels[os+".ID"]
		nodeOSmaj = labels[os+".VERSION_ID.major"]
		nodeOSmin = labels[os+".VERSION_ID.minor"]

		if len(nodeOSrel) == 0 || len(nodeOSmaj) == 0 {
			return "", "", "", fmt.Errorf("Cannot extract %s.*, is NFD running? Check node labels", os)
		}
	}
	// On OCP >4.7, we can use the NFD label  feature.node.kubernetes.io/system-os_release.RHEL_VERSION label.
	if rhelVersion, found := labels[os+".RHEL_VERSION"]; found && len(rhelVersion) == 3 {
		rhelMaj := rhelVersion[0:1]
		rhelMin := rhelVersion[2:]
		return "rhel" + rhelMaj, "rhel" + rhelVersion, rhelMaj + "." + rhelMin, nil
	}

	// On vanilla k8s and older NFD versions, we need RenderOperatingSystem
	return osversion.RenderOperatingSystem(nodeOSrel, nodeOSmaj, nodeOSmin)
}

func ClusterVersionAvailable() (bool, error) {

	clusterVersionAvailable, err := clients.Interface.HasResource(configv1.SchemeGroupVersion.WithResource("clusterversions"))
	if err != nil {
		return false, err
	}
	if !clusterVersionAvailable {
		log.Info("Warning: ClusterVersion API resource not available. Can be ignored on vanilla k8s.")
		return false, nil
	}
	return true, nil
}
