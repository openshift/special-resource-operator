package scheme_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift-psap/special-resource-operator/pkg/scheme"
	buildV1 "github.com/openshift/api/build/v1"
	imageV1 "github.com/openshift/api/image/v1"
	routev1 "github.com/openshift/api/route/v1"
	secv1 "github.com/openshift/api/security/v1"
	monitoringV1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestScheme(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Scheme Suite")
}

var _ = Describe("AddToScheme", func() {
	s := runtime.NewScheme()

	err := scheme.AddToScheme(s)
	Expect(err).NotTo(HaveOccurred())

	DescribeTable(
		"all GroupVersions should be registered",
		func(g schema.GroupVersion) {
			Expect(s.IsVersionRegistered(g)).To(BeTrue())
		},
		Entry(nil, routev1.GroupVersion),
		Entry(nil, secv1.GroupVersion),
		Entry(nil, buildV1.GroupVersion),
		Entry(nil, imageV1.GroupVersion),
		Entry(nil, monitoringV1.SchemeGroupVersion),
	)
})
