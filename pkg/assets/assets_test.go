package assets_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/openshift-psap/special-resource-operator/pkg/assets"
)

func TestAssets(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Assets Suite")
}

var _ = Describe("Assets", func() {
	var assetsInterface assets.Assets

	BeforeEach(func() {
		assetsInterface = assets.NewAssets()
	})

	Context("ValidStateName", func() {
		cases := []struct {
			input string
			valid bool
		}{
			{
				input: "0000_test.yaml",
				valid: true,
			},
			{
				input: "/path/to/0000_test.yaml",
				valid: true,
			},
			{
				input: "/0000_test.yaml",
				valid: true,
			},
			{
				input: "./0000_test.yaml",
				valid: true,
			},
			{
				input: "1234_test.yaml",
				valid: true,
			},
			{
				input: "123_test.yaml",
				valid: false,
			},
			{
				input: "12345_test.yaml",
				valid: false,
			},
			{
				input: "a1234_test.yaml",
				valid: false,
			},
			{
				input: "a1234_test.yml",
				valid: false,
			},
			{
				input: "abcd_test.yaml",
				valid: false,
			},
		}

		entries := make([]TableEntry, 0, len(cases))

		for _, c := range cases {
			entries = append(entries, Entry(c.input, c.input, c.valid))
		}

		DescribeTable(
			"all cases",
			func(input string, valid bool) {
				Expect(assetsInterface.ValidStateName(input)).To(Equal(valid))
			},
			entries...,
		)
	})
})
