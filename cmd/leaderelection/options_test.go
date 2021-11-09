package leaderelection

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestApplyOpenShiftOptions(t *testing.T) {
	checkValues := func(t *testing.T, opts *ctrl.Options) {
		t.Helper()

		assert.Equal(t, 137*time.Second, *opts.LeaseDuration)
		assert.Equal(t, 107*time.Second, *opts.RenewDeadline)
		assert.Equal(t, 26*time.Second, *opts.RetryPeriod)
	}

	t.Run("opts is nil", func(t *testing.T) {
		opts := ApplyOpenShiftOptions(nil)

		require.NotNil(t, opts)
		checkValues(t, opts)
	})

	t.Run("opts has other values", func(t *testing.T) {
		oneSecond := 1 * time.Second
		twoSeconds := 2 * time.Second
		threeSeconds := 3 * time.Second

		opts := &ctrl.Options{
			LeaseDuration: &oneSecond,
			RenewDeadline: &twoSeconds,
			RetryPeriod:   &threeSeconds,
		}

		opts = ApplyOpenShiftOptions(opts)

		checkValues(t, opts)
	})
}
