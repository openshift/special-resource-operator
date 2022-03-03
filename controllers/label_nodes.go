package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// If resource available, label the nodes according to the current state
// if e.g driver-container ready -> specialresource.openshift.io/driver-container:ready
func (r *SpecialResourceReconciler) labelNodesAccordingToState(ctx context.Context, log logr.Logger, nodeSelector map[string]string, key string) error {

	nodeList, err := r.KubeClient.GetNodesByLabels(ctx, nodeSelector)
	if err != nil {
		return fmt.Errorf("failed to get nodes with labels in labelNodesAccordingToState: %w", err)
	}

	for _, node := range nodeList.Items {
		node.Labels[key] = "Ready"

		if err = r.KubeClient.Update(ctx, &node); err != nil {
			if apierrors.IsForbidden(err) {
				return fmt.Errorf("forbidden - check Role, ClusterRole and Bindings: %w", err)
			}

			if apierrors.IsConflict(err) {
				return fmt.Errorf("node Conflict Label %s err %s", key, err)
			}

			log.Error(err, "Node Update", "label", key)
			return fmt.Errorf("couldn't Update Node: %w", err)
		}
	}

	return nil
}
