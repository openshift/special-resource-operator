package scheme_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
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

	gv := []schema.GroupVersion{
		routev1.GroupVersion,
		secv1.GroupVersion,
		buildV1.GroupVersion,
		imageV1.GroupVersion,
		monitoringV1.SchemeGroupVersion,
	}

	entries := make([]TableEntry, 0, len(gv))

	for _, g := range gv {
		entries = append(entries, Entry(g.String(), g))
	}

	DescribeTable(
		"all GroupVersions should be registered",
		func(g schema.GroupVersion) {
			Expect(s.IsVersionRegistered(g)).To(BeTrue())
		},
		entries...,
	)
})
