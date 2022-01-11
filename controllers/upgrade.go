package controllers

import (
	"context"
	"fmt"

	"github.com/openshift-psap/special-resource-operator/pkg/utils"
	ctrl "sigs.k8s.io/controller-runtime"
)

// SpecialResourceUpgrade upgrade special resources
func SpecialResourceUpgrade(ctx context.Context, r *SpecialResourceReconciler) (ctrl.Result, error) {
	log = r.Log.WithName(utils.Print("upgrade", utils.Blue))

	var err error

	nodeList, err := r.KubeClient.GetNodesByLabels(ctx, r.specialresource.Spec.NodeSelector)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get nodes: %w", err)
	}

	RunInfo.ClusterUpgradeInfo, err = r.ClusterInfo.GetClusterInfo(ctx, nodeList)
	if err != nil {
		return ctrl.Result{}, err
	}

	log.Info("TODO: preflight checks")

	return ctrl.Result{Requeue: false}, nil
}
