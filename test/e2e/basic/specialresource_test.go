package e2e

import (
	"testing"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	"github.com/openshift-psap/special-resource-operator/test/framework"
)

var (
	cs = framework.NewClientSet()
)

func TestSRO(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Special Resource Operator e2e tests: basic")
}
