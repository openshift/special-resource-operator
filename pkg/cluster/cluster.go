package cluster

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/utils"
	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

//go:generate mockgen -source=cluster.go -package=cluster -destination=mock_cluster_api.go

type Cluster interface {
	Version(context.Context) (string, string, error)
	VersionHistory(context.Context) ([]string, error)
	OSImageURL(context.Context) (string, error)
	OperatingSystem(*corev1.NodeList) (string, string, string, error)
}

func NewCluster(clients clients.ClientsInterface) Cluster {
	return &cluster{
		log:     zap.New(zap.UseDevMode(true)).WithName(utils.Print("cache", utils.Brown)),
		clients: clients,
	}
}

type cluster struct {
	log     logr.Logger
	clients clients.ClientsInterface
}

func (c *cluster) Version(ctx context.Context) (string, string, error) {

	available, err := c.clusterVersionAvailable()
	if err != nil {
		return "", "", err
	}
	if !available {
		return "", "", nil
	}

	version, err := c.clients.ClusterVersionGet(ctx, metav1.GetOptions{})
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

func (c *cluster) VersionHistory(ctx context.Context) ([]string, error) {

	stat := []string{}

	available, err := c.clusterVersionAvailable()
	if err != nil {
		return nil, err
	}
	if !available {
		return stat, nil
	}

	version, err := c.clients.ClusterVersionGet(ctx, metav1.GetOptions{})
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

func (c *cluster) OSImageURL(ctx context.Context) (string, error) {

	machineConfigAvailable, err := c.clients.HasResource(machinev1.SchemeGroupVersion.WithResource("machineconfigs"))
	if err != nil {
		return "", fmt.Errorf("Error discovering machineconfig API resource: %w", err)
	}
	if !machineConfigAvailable {
		c.log.Info("Warning: Could not find machineconfig API resource. Can be ignored on vanilla k8s.")
		return "", nil
	}

	cm := &unstructured.Unstructured{}
	cm.SetAPIVersion("v1")
	cm.SetKind("ConfigMap")

	namespacedName := types.NamespacedName{Namespace: "openshift-machine-config-operator", Name: "machine-config-osimageurl"}
	err = c.clients.Get(ctx, namespacedName, cm)
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
func (c *cluster) OperatingSystem(nodeList *corev1.NodeList) (string, string, string, error) {

	var nodeOSrel string
	var nodeOSmaj string
	var nodeOSmin string
	var labels map[string]string

	// Assuming all nodes are running the same os
	os := "feature.node.kubernetes.io/system-os_release"

	for _, node := range nodeList.Items {
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
	return utils.RenderOperatingSystem(nodeOSrel, nodeOSmaj, nodeOSmin)
}

func (c *cluster) clusterVersionAvailable() (bool, error) {

	clusterVersionAvailable, err := c.clients.HasResource(configv1.SchemeGroupVersion.WithResource("clusterversions"))
	if err != nil {
		return false, err
	}
	if !clusterVersionAvailable {
		c.log.Info("Warning: ClusterVersion API resource not available. Can be ignored on vanilla k8s.")
		return false, nil
	}
	return true, nil
}
