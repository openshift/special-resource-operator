package utils

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ParseOSInfo", func() {
	DescribeTable("test cases",
		func(osImage, expOut0, expOut1, expOut2 string, expErr bool) {
			out0, out1, out2, err := ParseOSInfo(osImage)

			if expErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(out0).To(Equal(expOut0))
				Expect(out1).To(Equal(expOut1))
				Expect(out2).To(Equal(expOut2))
			}
		},
		EntryDescription("OS=%q, => (%q, %q, %q) err=%t"),
		Entry(nil, "Red Hat Enterprise Linux CoreOS 410.810.202201102104-0 (Ootpa)", "4.10", "8.10", "8", false),
		Entry(nil, "Red Hat Enterprise Linux CoreOS 49.84.202201102104-0 (Ootpa)", "4.9", "8.4", "8", false),
		Entry(nil, "Red Hat Enterprise Linux CoreOS 48.83.202201102104-0 (Ootpa)", "4.8", "8.3", "8", false),
		Entry(nil, "Red Hat Enterprise Linux CoreOS 4.84.202201102104-0 (Ootpa)", "", "", "", true),
		Entry(nil, "Red Hat Enterprise Linux CoreOS 49.4.202201102104-0 (Ootpa)", "", "", "", true),
		Entry(nil, "Red Hat Enterprise Linux CoreOS 49.4 (Ootpa)", "", "", "", true),
	)
})
