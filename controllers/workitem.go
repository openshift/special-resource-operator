package controllers

import (
	"github.com/go-logr/logr"
	srov1beta1 "github.com/openshift/special-resource-operator/api/v1beta1"
	"github.com/openshift/special-resource-operator/pkg/runtime"
	"github.com/openshift/special-resource-operator/pkg/utils"
	"helm.sh/helm/v3/pkg/chart"
)

// WorkItem stores values required for current reconciliation
type WorkItem struct {
	// Log is a logger dedicated for specific SpecialResource constructed with its NamespacedName.
	Log logr.Logger

	// SpecialResource is currently reconciled object
	SpecialResource *srov1beta1.SpecialResource

	// AllSRs stores all of currently existing SpecialResources in the cluster.
	// It is used for resolving SpecialResource dependencies.
	AllSRs *srov1beta1.SpecialResourceList

	// Chart stores SpecialResource's chart
	Chart *chart.Chart

	// RunInfo contains information about the cluster.
	RunInfo *runtime.RuntimeInformation
}

func (wi *WorkItem) CreateForChild(child *srov1beta1.SpecialResource, c *chart.Chart) *WorkItem {
	return &WorkItem{
		Log:             wi.Log.WithName(utils.Print(child.GetName(), utils.Purple)),
		SpecialResource: child,
		AllSRs:          wi.AllSRs,
		Chart:           c,
	}
}
