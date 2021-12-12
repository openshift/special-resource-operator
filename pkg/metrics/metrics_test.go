package metrics

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	dto "github.com/prometheus/client_model/go"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	specialResourceCreateValue = 1
	completedStatesValue       = 2
	completedKindValue         = 2
	usedNodesValue             = 1

	sr         = "simple-kmod"
	state      = "templates/0000-buildconfig.yaml"
	kind       = "BuildConfig"
	name       = "simple-kmod-driver-build"
	namespace  = "openshift-special-resource-operator"
	nodes_list = "node1,node2,node3"
)

func TestMetrics(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Metrics Suite")
}

func findMetric(src []*dto.MetricFamily, query string) *dto.MetricFamily {
	for _, s := range src {
		if s.Name != nil && *s.Name == query {
			return s
		}
	}
	return nil
}

var _ = Describe("Metrics", func() {
	m := New()
	m.SetSpecialResourcesCreated(specialResourceCreateValue)
	m.SetCompletedState(sr, state, completedStatesValue)
	m.SetCompletedKind(sr, kind, name, namespace, completedKindValue)
	m.SetUsedNodes(sr, kind, name, namespace, nodes_list)

	It("correctly passes calls to the collectors", func() {
		expected := []struct {
			query string
			value int
		}{
			{createdSpecialResourcesQuery, specialResourceCreateValue},
			{completedStatesQuery, completedStatesValue},
			{completedKindQuery, completedKindValue},
			{usedNodesQuery, usedNodesValue},
		}

		data, err := metrics.Registry.Gather()
		Expect(err).NotTo(HaveOccurred())
		Expect(data).To(HaveLen(len(expected)))

		for _, e := range expected {
			m := findMetric(data, e.query)
			Expect(m).ToNot(BeNil(), "metric for %s could not be found", e.query)
			Expect(m.Metric).To(HaveLen(1))
			Expect(m.Metric[0].Gauge).ToNot(BeNil())
			Expect(m.Metric[0].Gauge.Value).ToNot(BeNil())
			Expect(*m.Metric[0].Gauge.Value).To(BeEquivalentTo(e.value))
		}
	})
})
