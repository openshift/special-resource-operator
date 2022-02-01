package utils

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RenderOperatingSystem", func() {
	DescribeTable("test cases",
		func(rel, maj, min, expOut0, expOut1, expOut2 string, expErr bool) {
			out0, out1, out2, err := RenderOperatingSystem(rel, maj, min)

			if expErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(out0).To(Equal(expOut0))
				Expect(out1).To(Equal(expOut1))
				Expect(out2).To(Equal(expOut2))
			}
		},
		EntryDescription("rel=%q, maj=%q, min=%q => (%q, %q, %q) err=%t"),
		Entry(nil, "rhcos", "3", "0", "rhcos3", "rhcos3.0", "3.0", false),
		Entry(nil, "rhcos", "4", "2", "rhel8", "rhel8.0", "8.0", false),
		Entry(nil, "rhcos", "4", "4", "rhel8", "rhel8.1", "8.1", false),
		Entry(nil, "rhcos", "4", "5", "rhel8", "rhel8.2", "8.2", false),
		Entry(nil, "rhcos", "4", "6", "rhel8", "rhel8.2", "8.2", false),
		Entry(nil, "rhcos", "4", "7", "rhel8", "rhel8.4", "8.4", false),
		Entry(nil, "rhcos", "4", "8", "rhel8", "rhel8.4", "8.4", false),
		Entry(nil, "rhcos", "4", "8", "rhel8", "rhel8.4", "8.4", false),
		Entry(nil, "rhcos", "5", "", "rhcos5", "rhcos5", "5", false),
		Entry(nil, "rhcos", "5", "1", "rhcos5", "rhcos5.1", "5.1", false),
	)
})
