package controllers

import (
	"context"
	"strings"

	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"github.com/openshift-psap/special-resource-operator/pkg/exit"
	"github.com/pkg/errors"
	errs "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	client "sigs.k8s.io/controller-runtime/pkg/client"
)

type nodeUpgradeVersion struct {
	rhelVersion    string
	clusterVersion string
}

// SpecialResourceUpgrade upgrade special resources
func SpecialResourceUpgrade(r *SpecialResourceReconciler, req ctrl.Request) (ctrl.Result, error) {

	var err error

	log = r.Log.WithName(color.Print("upgrade", color.Red))

	log.Info("Get Node List")
	runInfo.Node.list, err = cacheNodes(r, false)
	exit.OnError(errs.Wrap(err, "Failed to cache nodes"))

	log.Info("Get Upgrade Info")
	runInfo.ClusterUpgradeInfo, err = getUpgradeInfo()
	exit.OnError(errs.Wrap(err, "Failed to get upgrade info"))

	log.Info("TODO: preflight checks")

	return ctrl.Result{Requeue: false}, nil
}

func cacheNodes(r *SpecialResourceReconciler, force bool) (*unstructured.UnstructuredList, error) {

	// The initial list is what we're working with
	// a SharedInformer will update the list of nodes if
	// more nodes join the cluster.
	cached := int64(len(runInfo.Node.list.Items))
	if cached == runInfo.Node.count && !force {
		return runInfo.Node.list, nil
	}

	runInfo.Node.list.SetAPIVersion("v1")
	runInfo.Node.list.SetKind("NodeList")

	opts := []client.ListOption{}

	// Only filter if we have a selector set, otherwise zero nodes will be
	// returned and no labels can be extracted. Set the default worker label
	// otherwise.
	if len(r.specialresource.Spec.Node.Selector) > 0 {
		opts = append(opts, client.MatchingLabels{r.specialresource.Spec.Node.Selector: "true"})
	} else {
		opts = append(opts, client.MatchingLabels{"node-role.kubernetes.io/worker": ""})
	}

	err := r.List(context.TODO(), runInfo.Node.list, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "Client cannot get NodeList")
	}

	return runInfo.Node.list, err
}

func getUpgradeInfo() (map[string]nodeUpgradeVersion, error) {

	var found bool
	var info = make(map[string]nodeUpgradeVersion)

	// Assuming all nodes are running the same kernel version,
	// one could easily add driver-kernel-versions for each node.
	for _, node := range runInfo.Node.list.Items {

		var rhelVersion string
		var kernelFullVersion string
		var clusterVersion string

		labels := node.GetLabels()
		// We only need to check for the key, the value
		// is available if the key is there
		short := "feature.node.kubernetes.io/kernel-version.full"
		if kernelFullVersion, found = labels[short]; !found {
			return nil, errs.New("Label " + short + " not found is NFD running? Check node labels")
		}

		short = "feature.node.kubernetes.io/system-os_release.RHEL_VERSION"
		if rhelVersion, found = labels[short]; !found {
			return nil, errs.New("Label " + short + " not found is NFD running? Check node labels")
		}

		short = "feature.node.kubernetes.io/system-os_release.VERSION_ID"
		if clusterVersion, found = labels[short]; !found {
			return nil, errs.New("Label " + short + " not found is NFD running? Check node labels")
		}

		info[kernelFullVersion] = nodeUpgradeVersion{rhelVersion: rhelVersion, clusterVersion: clusterVersion}
	}

	return info, nil
}

func setKernelVersionNodeAffinity(obj *unstructured.Unstructured) error {

	if strings.Compare(obj.GetKind(), "DaemonSet") == 0 {
		if err := kernelVersionNodeAffinity(obj, "spec", "template", "spec", "nodeSelector"); err != nil {
			return errs.Wrap(err, "Cannot setup DaemonSet kernel version affinity")
		}
	}
	if strings.Compare(obj.GetKind(), "Pod") == 0 {
		if err := kernelVersionNodeAffinity(obj, "spec", "nodeSelector"); err != nil {
			return errs.Wrap(err, "Cannot setup Pod kernel version affinity")
		}
	}
	if strings.Compare(obj.GetKind(), "BuildConfig") == 0 {
		if err := kernelVersionNodeAffinity(obj, "spec", "nodeSelector"); err != nil {
			return errs.Wrap(err, "Cannot setup BuildConfig kernel version affinity")
		}
	}

	return nil
}

func kernelVersionNodeAffinity(obj *unstructured.Unstructured, fields ...string) error {

	nodeSelector, found, err := unstructured.NestedMap(obj.Object, fields...)
	exit.OnError(err)

	if !found {
		nodeSelector = make(map[string]interface{})
	}

	nodeSelector["feature.node.kubernetes.io/kernel-version.full"] = runInfo.KernelFullVersion

	if err := unstructured.SetNestedMap(obj.Object, nodeSelector, fields...); err != nil {
		return errs.Wrap(err, "Cannot update nodeSelector")
	}

	return nil
}
