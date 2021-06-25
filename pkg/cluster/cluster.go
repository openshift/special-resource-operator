package cluster

import (
	"context"
	"strings"

	"github.com/openshift-psap/special-resource-operator/pkg/cache"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/exit"
	"github.com/openshift-psap/special-resource-operator/pkg/osversion"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

func Version() (string, string, error) {

	version, err := clients.Interface.ClusterVersions().Get(context.TODO(), "version", metav1.GetOptions{})
	if err != nil {
		return "", "", errors.Wrap(err, "ConfigClient unable to get ClusterVersions")
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
	version, err := clients.Interface.ClusterVersions().Get(context.TODO(), "version", metav1.GetOptions{})
	if err != nil {
		return stat, errors.Wrap(err, "ConfigClient unable to get ClusterVersions")
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

	cm := &unstructured.Unstructured{}
	cm.SetAPIVersion("v1")
	cm.SetKind("ConfigMap")

	namespacedName := types.NamespacedName{Namespace: "openshift-machine-config-operator", Name: "machine-config-osimageurl"}
	err := clients.Interface.Get(context.TODO(), namespacedName, cm)
	if apierrors.IsNotFound(err) {
		return "", errors.Wrap(err, "ConfigMap machine-config-osimageurl -n  openshift-machine-config-operator not found")
	}

	osImageURL, found, err := unstructured.NestedString(cm.Object, "data", "osImageURL")
	exit.OnErrorOrNotFound(found, err)

	return osImageURL, nil
}

func OperatingSystem() (string, string, string, error) {

	var nodeOSrel string
	var nodeOSmaj string
	var nodeOSmin string

	// Assuming all nodes are running the same os
	os := "feature.node.kubernetes.io/system-os_release"

	for _, node := range cache.Node.List.Items {
		labels := node.GetLabels()
		nodeOSrel = labels[os+".ID"]
		nodeOSmaj = labels[os+".VERSION_ID.major"]
		nodeOSmin = labels[os+".VERSION_ID.minor"]

		if len(nodeOSrel) == 0 || len(nodeOSmaj) == 0 {
			return "", "", "", errors.New("Cannot extract " + os + ".*, is NFD running? Check node labels")
		}
	}

	return osversion.RenderOperatingSystem(nodeOSrel, nodeOSmaj, nodeOSmin)
}
