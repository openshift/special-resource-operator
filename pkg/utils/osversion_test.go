package utils

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
)

type testCase struct {
	rel        string
	maj        string
	min        string
	out0       string
	out1       string
	out2       string
	expectsErr bool
}

func (tc *testCase) String() string {
	str := fmt.Sprintf(
		"%s %s.%s => [%s, %s, %s]",
		tc.rel,
		tc.maj,
		tc.min,
		tc.out0,
		tc.out1,
		tc.out2)

	if tc.expectsErr {
		str += " (error expected)"
	}

	return str
}

var _ = Describe("RenderOperatingSystem", func() {
	cases := []*testCase{
		{
			rel: "rhcos", maj: "3", min: "0",
			out0: "rhcos3", out1: "rhcos3.0", out2: "3.0",
			expectsErr: false,
		},
		{
			rel: "rhcos", maj: "4", min: "2",
			out0: "rhel8", out1: "rhel8.0", out2: "8.0",
			expectsErr: false,
		},
		{
			rel: "rhcos", maj: "4", min: "4",
			out0: "rhel8", out1: "rhel8.1", out2: "8.1",
			expectsErr: false,
		},
		{
			rel: "rhcos", maj: "4", min: "5",
			out0: "rhel8", out1: "rhel8.2", out2: "8.2",
			expectsErr: false,
		},
		{
			rel: "rhcos", maj: "4", min: "6",
			out0: "rhel8", out1: "rhel8.2", out2: "8.2",
			expectsErr: false,
		},
		{
			rel: "rhcos", maj: "4", min: "7",
			out0: "rhel8", out1: "rhel8.4", out2: "8.4",
			expectsErr: false,
		},
		{
			rel: "rhcos", maj: "4", min: "8",
			out0: "rhel8", out1: "rhel8.4", out2: "8.4",
			expectsErr: false,
		},
		{
			rel: "rhcos", maj: "4", min: "8",
			out0: "rhel8", out1: "rhel8.4", out2: "8.4",
			expectsErr: false,
		},
		{
			rel: "rhcos", maj: "5", min: "",
			out0: "rhcos5", out1: "rhcos5", out2: "5",
			expectsErr: false,
		},
		{
			rel: "rhcos", maj: "5", min: "1",
			out0: "rhcos5", out1: "rhcos5.1", out2: "5.1",
			expectsErr: false,
		},
	}

	entries := make([]TableEntry, 0, len(cases))

	for _, c := range cases {
		entries = append(
			entries,
			Entry(c.String(), c),
		)
	}

	DescribeTable("test cases",
		func(c *testCase) {
			out0, out1, out2, err := RenderOperatingSystem(c.rel, c.maj, c.min)

			if c.expectsErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(out0).To(Equal(c.out0))
				Expect(out1).To(Equal(c.out1))
				Expect(out2).To(Equal(c.out2))
			}
		},
		entries...,
	)
})
