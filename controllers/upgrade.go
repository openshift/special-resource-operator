package controllers

import (
	"fmt"

	"github.com/openshift-psap/special-resource-operator/pkg/cache"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"github.com/openshift-psap/special-resource-operator/pkg/exit"
	"github.com/openshift-psap/special-resource-operator/pkg/upgrade"
	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
)

// SpecialResourceUpgrade upgrade special resources
func SpecialResourceUpgrade(r *SpecialResourceReconciler, req ctrl.Request) (ctrl.Result, error) {

	log = r.Log.WithName(color.Print("upgrade", color.Blue))

	err := cache.Nodes(r.specialresource.Spec.NodeSelector, false)
	exit.OnError(errors.Wrap(err, "Failed to cache nodes"))

	RunInfo.ClusterUpgradeInfo, err = upgrade.ClusterInfo()
	exit.OnError(err)

	fmt.Printf("DRIVERTOOLKITVERSION %+v\n", RunInfo.ClusterUpgradeInfo)

	log.Info("TODO: preflight checks")

	return ctrl.Result{Requeue: false}, nil
}
