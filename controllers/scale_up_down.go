package controllers

import (
	"context"
	"fmt"

	"github.com/openshift-psap/special-resource-operator/pkg/cache"
	"github.com/openshift-psap/special-resource-operator/pkg/state"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// If resource available, label the nodes according to the current state
// if e.g driver-container ready -> specialresource.openshift.io/driver-container:ready
func (r *SpecialResourceReconciler) labelNodesAccordingToState(ctx context.Context, nodeSelector map[string]string) error {
	var err error

	if err = r.NodesCacher.Nodes(ctx, nodeSelector, true); err != nil {
		return fmt.Errorf("could not cache nodes for state change: %w", err)
	}

	for _, node := range cache.Node.List.Items {
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
				if err := r.NodesCacher.Nodes(ctx, nodeSelector, true); err != nil {
					return fmt.Errorf("could not cache nodes for api conflict: %w", err)
				}

				return fmt.Errorf("node Conflict Label %s err %s", state.CurrentName, err)
			}

			log.Error(err, "Node Update", "label", state.CurrentName)
			return fmt.Errorf("couldn't Update Node: %w", err)
		}

		log.Info("NODE", "Setting Label ", state.CurrentName, "on ", updated.GetName())

	}

	return nil
}
