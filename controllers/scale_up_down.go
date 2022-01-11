package controllers

import (
	"context"
	"fmt"

	"github.com/openshift-psap/special-resource-operator/pkg/state"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// If resource available, label the nodes according to the current state
// if e.g driver-container ready -> specialresource.openshift.io/driver-container:ready
func (r *SpecialResourceReconciler) labelNodesAccordingToState(ctx context.Context, nodeSelector map[string]string) error {

	nodeList, err := r.KubeClient.GetNodesByLabels(ctx, r.specialresource.Spec.NodeSelector)
	if err != nil {
		return fmt.Errorf("failed to get nodes with labels in labelNodesAccordingToState: %w", err)
	}

	for _, node := range nodeList.Items {
		labels := node.GetLabels()

		// Label missing update the Node to advance to the next state
		updated := node.DeepCopy()

		labels[state.CurrentName] = "Ready"

		updated.SetLabels(labels)

		if err = r.KubeClient.Update(ctx, updated); err != nil {
			if apierrors.IsForbidden(err) {
				return fmt.Errorf("forbidden - check Role, ClusterRole and Bindings: %w", err)
			}

			if apierrors.IsConflict(err) {
				return fmt.Errorf("node Conflict Label %s err %s", state.CurrentName, err)
			}

			log.Error(err, "Node Update", "label", state.CurrentName)
			return fmt.Errorf("couldn't Update Node: %w", err)
		}

		log.Info("NODE", "Setting Label ", state.CurrentName, "on ", updated.GetName())

	}

	return nil
}
