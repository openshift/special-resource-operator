package assets_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
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
		DescribeTable(
			"all cases",
			func(input string, valid bool) {
				Expect(assetsInterface.ValidStateName(input)).To(Equal(valid))
			},
			EntryDescription("%s: %t"),
			Entry(nil, "0000_test.yaml", true),
			Entry(nil, "/path/to/0000_test.yaml", true),
			Entry(nil, "/0000_test.yaml", true),
			Entry(nil, "./0000_test.yaml", true),
			Entry(nil, "1234_test.yaml", true),
			Entry(nil, "123_test.yaml", false),
			Entry(nil, "12345_test.yaml", false),
			Entry(nil, "a1234_test.yaml", false),
			Entry(nil, "a1234_test.yml", false),
			Entry(nil, "abcd_test.yaml", false),
		)
	})
})
