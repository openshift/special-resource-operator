package controllers

import (
	"fmt"

	"github.com/openshift-psap/special-resource-operator/pkg/cache"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"github.com/openshift-psap/special-resource-operator/pkg/upgrade"
	ctrl "sigs.k8s.io/controller-runtime"
)

// SpecialResourceUpgrade upgrade special resources
func SpecialResourceUpgrade(r *SpecialResourceReconciler, req ctrl.Request) (ctrl.Result, error) {
	log = r.Log.WithName(color.Print("upgrade", color.Blue))

	var err error

	if err = cache.Nodes(r.specialresource.Spec.NodeSelector, false); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to cache nodes: %w", err)
	}

	RunInfo.ClusterUpgradeInfo, err = upgrade.ClusterInfo()
	if err != nil {
		return ctrl.Result{}, err
	}

	log.Info("TODO: preflight checks")

	return ctrl.Result{Requeue: false}, nil
}
