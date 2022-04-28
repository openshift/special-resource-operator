package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// When adding metric names, see https://prometheus.io/docs/practices/naming/#metric-names
const (
	specialResourcesCreatedQuery = "sro_managed_resources_total"
	completedStatesQuery         = "sro_states_completed_info"
	completedKindQuery           = "sro_kind_completed_info"
	usedNodesQuery               = "sro_used_nodes"
)

var (
	//#TODO set the metric
	specialResourcesCreated = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: specialResourcesCreatedQuery,
			Help: "Number of specialresources created",
		},
	)
	completedStates = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: completedStatesQuery,
			Help: "For a given specialresource and state, 1 if the state is completed, 0 if it is not.",
		},
		[]string{"specialresource", "state"},
	)
	completedKinds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: completedKindQuery,
			Help: "For a given specialresource,kind,name and namespace, 1 if the state is completed, 0 if it is not.",
		},
		[]string{"specialresource", "kind", "name", "namespace"},
	)
	usedNodes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: usedNodesQuery,
			Help: "Nodes that the deployments/daemonsets' pods are running on",
		},
		[]string{"cr", "kind", "name", "namespace", "nodes"},
	)
)

// SetCompletedState set completed states
func SetCompletedState(specialResource string, state string, value int) {
	completedStates.WithLabelValues(specialResource, state).Set(float64(value))
}

// DeleteCompleteStates delete metric complete states
func DeleteCompleteStates(specialResource string, state string) {
	completedStates.DeleteLabelValues(specialResource, state)
}

// SetSpecialResourcesCreated set number of created states
func SetSpecialResourcesCreated(value int) {
	specialResourcesCreated.Set(float64(value))
}

func SetCompletedKind(specialResource, kind, name, namespace string, value int) {
	completedKinds.WithLabelValues(specialResource, kind, name, namespace).Set(float64(value))
}

func SetUsedNodes(crName, kind, name, namespace, nodes string) {
	usedNodes.WithLabelValues(crName, kind, name, namespace, nodes).Set(float64(1))
}

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(
		specialResourcesCreated,
		completedStates,
		completedKinds,
		usedNodes,
	)

}
