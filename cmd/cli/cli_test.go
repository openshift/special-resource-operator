package cli_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift/special-resource-operator/cmd/cli"
)

func TestCli(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cli Suite")
}

var _ = Describe("Cli", func() {
	Context("ParseCommandLine", func() {
		It("should return the default values", func() {
			cl, err := cli.ParseCommandLine("test", nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(cl.EnableLeaderElection).To(BeFalse())
			Expect(cl.MetricsAddr).To(Equal(":8080"))
		})

		It("should set all flags correctly", func() {
			const metricsAddr = "1.2.3.4:5678"

			expected := &cli.CommandLine{
				EnableLeaderElection: true,
				MetricsAddr:          metricsAddr,
			}

			args := []string{
				"--enable-leader-election",
				"--metrics-addr", metricsAddr,
			}

			cl, err := cli.ParseCommandLine("test", args)
			Expect(err).NotTo(HaveOccurred())

			Expect(cl).To(Equal(expected))
		})
	})
})
