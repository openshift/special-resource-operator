package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// When adding metric names, see https://prometheus.io/docs/practices/naming/#metric-names
const (
	createdSpecialResourcesQuery = "sro_managed_resources_total"
	completedStatesQuery         = "sro_states_completed_info"
	completedKindQuery           = "sro_kind_completed_info"
	usedNodesQuery               = "sro_used_nodes"
)

var (
	//#TODO set the metric
	createdSpecialResources = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: createdSpecialResourcesQuery,
			Help: "Number of created SpecialResources",
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

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(
		completedStates,
		createdSpecialResources,
		completedKinds,
		usedNodes,
	)
}

//go:generate mockgen -source=metrics.go -package=metrics -destination=mock_metrics_api.go

// Metrics is an interface representing a prometheus client for the Special Resource Operator
type Metrics interface {
	SetSpecialResourcesCreated(value int)
	SetCompletedState(specialResource, state string, value int)
	SetCompletedKind(specialResource, kind, name, namespace string, value int)
	SetUsedNodes(crName, kind, name, namespace, nodes string)
}

func New() Metrics {
	return &metricsImpl{}
}

type metricsImpl struct{}

func (m *metricsImpl) SetSpecialResourcesCreated(value int) {
	createdSpecialResources.Set(float64(value))
}

func (m *metricsImpl) SetCompletedState(specialResource, state string, value int) {
	completedStates.WithLabelValues(specialResource, state).Set(float64(value))
}

func (m *metricsImpl) SetCompletedKind(specialResource, kind, name, namespace string, value int) {
	completedKinds.WithLabelValues(specialResource, kind, name, namespace).Set(float64(value))
}

func (m *metricsImpl) SetUsedNodes(crName, kind, name, namespace, nodes string) {
	usedNodes.WithLabelValues(crName, kind, name, namespace, nodes).Set(float64(1))
}
