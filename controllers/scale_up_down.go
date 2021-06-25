package controllers

import (
	"context"

	"fmt"

	"github.com/openshift-psap/special-resource-operator/pkg/cache"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/state"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// If resource available, label the nodes according to the current state
// if e.g driver-container ready -> specialresource.openshift.io/driver-container:ready
func labelNodesAccordingToState(obj *unstructured.Unstructured, r *SpecialResourceReconciler) error {

	if obj.GetKind() != "DaemonSet" {
		return nil
	}

	var err error

	if err = cache.Nodes(r.specialresource.Spec.NodeSelector, true); err != nil {
		return errors.Wrap(err, "Could not cache nodes for state change")
	}

	for _, node := range cache.Node.List.Items {

		labels := node.GetLabels()

		// Label missing update the Node to advance to the next state
		updated := node.DeepCopy()

		labels[state.CurrentName] = "Ready"

		updated.SetLabels(labels)

		err := clients.Interface.Update(context.TODO(), updated)
		if apierrors.IsForbidden(err) {
			return errors.Wrap(err, "forbidden check Role, ClusterRole and Bindings for operator %s")
		}
		if apierrors.IsConflict(err) {
			var err error

			if err = cache.Nodes(r.specialresource.Spec.NodeSelector, true); err != nil {
				return errors.Wrap(err, "Could not cache nodes for api conflict")
			}

			return fmt.Errorf("node Conflict Label %s err %s", state.CurrentName, err)
		}

		if err != nil {
			log.Error(err, "Node Update", "label", state.CurrentName)
			return fmt.Errorf("couldn't Update Node")
		}

		log.Info("NODE", "Setting Label ", state.CurrentName, "on ", updated.GetName())

		operatorStatusUpdate(obj, r, state.CurrentName)

	}
	return nil
}
