package controllers

import (
	srov1beta1 "github.com/openshift-psap/special-resource-operator/api/v1beta1"
	"github.com/openshift-psap/special-resource-operator/pkg/runtime"
	"helm.sh/helm/v3/pkg/chart"
)

// WorkItem stores values required for current reconciliation
type WorkItem struct {
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
		SpecialResource: child,
		AllSRs:          wi.AllSRs,
		Chart:           c,
	}
}
