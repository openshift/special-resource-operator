package slice_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/openshift-psap/special-resource-operator/pkg/slice"
	"helm.sh/helm/v3/pkg/chart"
)

func TestSlice(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Slice Suite")
}

var _ = Describe("Find", func() {
	s := []string{"a", "b", "c", "d"}

	DescribeTable(
		"should work as expected",
		func(v string, ret int) {
			Expect(slice.Find(s, v)).To(Equal(ret))
		},
		Entry("c: in the slice", "c", 2),
		Entry("z: not in the slice", "z", len(s)),
	)
})

var _ = Describe("Contains", func() {
	s := []string{"a", "b", "c", "d"}

	DescribeTable(
		"should return the expected boolean",
		func(v string, m types.GomegaMatcher) {
			Expect(slice.Contains(s, v)).To(m)
		},
		Entry("a", "a", BeTrue()),
		Entry("a", "z", BeFalse()),
	)
})

var _ = Describe("FindCRFile", func() {
	files := []*chart.File{
		{Name: "chart0.yaml"},
		{Name: "chart1.yaml"},
	}

	DescribeTable(
		"should return the expected index",
		func(name string, index int) {
			Expect(slice.FindCRFile(files, name)).To(Equal(index))
		},
		Entry("chart1: in the slice", "chart1", 1),
		Entry("chart99: not in the slice", "chart99", -1),
	)
})

var _ = Describe("Insert", func() {
	var a []string

	BeforeEach(func() {
		a = []string{"a", "b"}
	})

	DescribeTable(
		"should return the expected slice",
		func(idx int, expected []string) {
			Expect(slice.Insert(a, idx, "c")).To(Equal(expected))
		},
		Entry("at 0", 0, []string{"c", "a", "b"}),
		Entry("at 1", 1, []string{"a", "c", "b"}),
		Entry("at 2", 2, []string{"a", "b", "c"}),
	)
})