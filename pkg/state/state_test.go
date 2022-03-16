package state_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"helm.sh/helm/v3/pkg/chart"

	"github.com/openshift/special-resource-operator/pkg/state"
)

func TestState(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "State Suite")
}

var _ = Describe("GenerateName", func() {
	AfterEach(func() {
		state.CurrentName = ""
	})

	It("should generate the correct name", func() {
		f := &chart.File{Name: "/path/to/test.json"}

		state.GenerateName(f, "some-sr")

		Expect(state.CurrentName).To(Equal("specialresource.openshift.io/state-some-sr-test"))
	})
})
