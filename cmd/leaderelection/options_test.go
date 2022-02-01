package leaderelection_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift-psap/special-resource-operator/cmd/leaderelection"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestLeaderelection(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Leaderelection Suite")
}

var _ = Describe("Leaderelection", func() {
	Context("ApplyOpenShiftOptions", func() {
		checkValues := func(opts *ctrl.Options) {
			Expect(*opts.LeaseDuration).To(Equal(137 * time.Second))
			Expect(*opts.RenewDeadline).To(Equal(107 * time.Second))
			Expect(*opts.RetryPeriod).To(Equal(26 * time.Second))
		}

		It("should set correct defaults when opts is nil", func() {
			opts := leaderelection.ApplyOpenShiftOptions(nil)

			Expect(opts).NotTo(BeNil())
			checkValues(opts)
		})

		It("opts has other values", func() {
			oneSecond := 1 * time.Second
			twoSeconds := 2 * time.Second
			threeSeconds := 3 * time.Second

			opts := &ctrl.Options{
				LeaseDuration: &oneSecond,
				RenewDeadline: &twoSeconds,
				RetryPeriod:   &threeSeconds,
			}

			opts = leaderelection.ApplyOpenShiftOptions(opts)

			checkValues(opts)
		})
	})

})
