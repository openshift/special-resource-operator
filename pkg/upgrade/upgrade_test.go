package upgrade_test

import (
	"testing"

	"github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/extensions/table"
	"github.com/onsi/gomega"
	"github.com/openshift-psap/special-resource-operator/pkg/registry"
	"github.com/openshift-psap/special-resource-operator/pkg/upgrade"
)

var _ = ginkgo.Describe("pkg/upgrade", func() {
	ginkgo.Context("UpdateInfo", func() {

		defaultKernelFullVersion := "4.18.0-305.19.1.el8_4"
		defaultKernelFullVersionArch := defaultKernelFullVersion + ".x86_64"
		defaultRTKernelFullVersion := "4.18.0-305.19.1.rt7.91.el8_4"
		defaultRTKernelFullVersionArch := defaultRTKernelFullVersion + ".x86_64"
		defaultOSVersion := "8.4"
		defaultClusterVersion := "4.9"

		defaultDTKInput := registry.DriverToolkitEntry{
			KernelFullVersion:   defaultKernelFullVersion,
			RTKernelFullVersion: defaultRTKernelFullVersion,
			OSVersion:           defaultOSVersion,
		}
		defaultImageURLInput := "quay.io/somerepo/someimage@sha256:1234567890abcdef"

		ginkgo.When("versions match", func() {
			testCases := []table.TableEntry{
				table.Entry(
					"Regular and RT Kernel",
					map[string]upgrade.NodeVersion{
						defaultKernelFullVersionArch: upgrade.NodeVersion{
							OSVersion:      defaultOSVersion,
							ClusterVersion: defaultClusterVersion,
						},
						defaultRTKernelFullVersionArch: upgrade.NodeVersion{
							OSVersion:      defaultOSVersion,
							ClusterVersion: defaultClusterVersion,
						},
					},
					map[string]upgrade.NodeVersion{
						defaultKernelFullVersionArch: upgrade.NodeVersion{
							OSVersion:      defaultOSVersion,
							ClusterVersion: defaultClusterVersion,
							DriverToolkit: registry.DriverToolkitEntry{
								ImageURL:            defaultImageURLInput,
								KernelFullVersion:   defaultKernelFullVersionArch,
								RTKernelFullVersion: defaultRTKernelFullVersionArch,
								OSVersion:           defaultOSVersion,
							},
						},
						defaultRTKernelFullVersionArch: upgrade.NodeVersion{
							OSVersion:      defaultOSVersion,
							ClusterVersion: defaultClusterVersion,
							DriverToolkit: registry.DriverToolkitEntry{
								ImageURL:            defaultImageURLInput,
								KernelFullVersion:   defaultKernelFullVersionArch,
								RTKernelFullVersion: defaultRTKernelFullVersionArch,
								OSVersion:           defaultOSVersion,
							},
						},
					},
				),
				table.Entry(
					"Regular kernel only",
					map[string]upgrade.NodeVersion{
						defaultKernelFullVersionArch: upgrade.NodeVersion{
							OSVersion:      defaultOSVersion,
							ClusterVersion: defaultClusterVersion,
						},
					},
					map[string]upgrade.NodeVersion{
						defaultKernelFullVersionArch: upgrade.NodeVersion{
							OSVersion:      defaultOSVersion,
							ClusterVersion: defaultClusterVersion,
							DriverToolkit: registry.DriverToolkitEntry{
								ImageURL:            defaultImageURLInput,
								KernelFullVersion:   defaultKernelFullVersionArch,
								RTKernelFullVersion: defaultRTKernelFullVersionArch,
								OSVersion:           defaultOSVersion,
							},
						},
					},
				),
				table.Entry(
					"RT kernel only",
					map[string]upgrade.NodeVersion{
						defaultRTKernelFullVersionArch: upgrade.NodeVersion{
							OSVersion:      defaultOSVersion,
							ClusterVersion: defaultClusterVersion,
						},
					},
					map[string]upgrade.NodeVersion{
						defaultRTKernelFullVersionArch: upgrade.NodeVersion{
							OSVersion:      defaultOSVersion,
							ClusterVersion: defaultClusterVersion,
							DriverToolkit: registry.DriverToolkitEntry{
								ImageURL:            defaultImageURLInput,
								KernelFullVersion:   defaultKernelFullVersionArch,
								RTKernelFullVersion: defaultRTKernelFullVersionArch,
								OSVersion:           defaultOSVersion,
							},
						},
					},
				),
			}
			table.DescribeTable("test table", func(input map[string]upgrade.NodeVersion, expected map[string]upgrade.NodeVersion) {
				info, err := upgrade.UpdateInfo(input, defaultDTKInput, defaultImageURLInput)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(info).Should(gomega.Equal(expected))
			}, testCases...)
		})

		ginkgo.When("versions dont match", func() {
			testCases := []table.TableEntry{
				table.Entry(
					"Regular kernel mismatch",
					map[string]upgrade.NodeVersion{
						defaultKernelFullVersionArch: upgrade.NodeVersion{
							OSVersion:      "8.1",
							ClusterVersion: defaultClusterVersion,
						},
					},
					"OSVersion mismatch NFD: 8.1 vs. DTK: 8.4",
				),
				table.Entry(
					"RT kernel mismatch",
					map[string]upgrade.NodeVersion{
						defaultRTKernelFullVersionArch: upgrade.NodeVersion{
							OSVersion:      "8.2",
							ClusterVersion: defaultClusterVersion,
						},
					},
					"OSVersion mismatch NFD: 8.2 vs. DTK: 8.4",
				),
			}
			table.DescribeTable("test table", func(input map[string]upgrade.NodeVersion, expected string) {
				_, err := upgrade.UpdateInfo(input, defaultDTKInput, defaultImageURLInput)
				gomega.Expect(err).To(gomega.HaveOccurred())
				gomega.Expect(err.Error()).To(gomega.Equal(expected))
			}, testCases...)
		})
	})
})

func TestPkgUpgrade(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "pkg/upgrade Unit tests")
}
