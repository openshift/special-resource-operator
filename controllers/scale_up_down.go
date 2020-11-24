package controllers

import (
	"context"

	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// If resource available, label the nodes according to the current state
// if e.g driver-container ready -> specialresource.openshift.io/driver-container:ready
func labelNodesAccordingToState(obj *unstructured.Unstructured, r *SpecialResourceReconciler) error {

	if obj.GetKind() != "DaemonSet" {
		return nil
	}

	cacheNodes(r, true)

	hw := r.specialresource.Name
	st := runInfo.StateName

	var stateLabels = map[string]map[string]string{
		"driver-container":   {st.DriverContainer + "-" + hw: "ready"},
		"runtime-enablement": {st.RuntimeEnablement + "-" + hw: "ready"},
		"device-plugin":      {st.DevicePlugin + "-" + hw: "ready"},
		"device-monitoring":  {st.DeviceMonitoring + "-" + hw: "ready"},
	}

	for _, node := range node.list.Items {
		labels := node.GetLabels()

		state := obj.GetAnnotations()["specialresource.openshift.io/state"]

		stateLabel, found := stateLabels[state]
		if !found {
			return nil
		}

		for k := range stateLabel {

			_, found := labels[k]
			if found {
				log.Info("Label", "found", stateLabel, "on ", node.GetName())
				updateStatus(obj, r, stateLabel)
				continue
			}
			// Label missing update the Node to advance to the next state
			updated := node.DeepCopy()

			labels[k] = "ready"

			updated.SetLabels(labels)

			err := r.Update(context.TODO(), updated)
			if apierrors.IsForbidden(err) {
				return fmt.Errorf("Forbidden check Role, ClusterRole and Bindings for operator %s", err)
			}
			if apierrors.IsConflict(err) {
				cacheNodes(r, true)
				return fmt.Errorf("Node Conflict Label %s err %s", stateLabel, err)
			}

			if err != nil {
				log.Error(err, "Node Update", "label", stateLabel)
				return fmt.Errorf("Couldn't Update Node")
			}

			log.Info("NODE", "Setting Label ", stateLabel, "on ", updated.GetName())

			updateStatus(obj, r, stateLabel)
		}
	}
	return nil
}
