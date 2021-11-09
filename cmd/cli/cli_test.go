package cli

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseCommandLine(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		cl, err := ParseCommandLine("test", nil)
		assert.NoError(t, err)

		assert.False(t, cl.EnableLeaderElection)
		assert.Equal(t, ":8080", cl.MetricsAddr)
		assert.Equal(t, 137*time.Second, cl.LeaderElectionLeaseDuration)
	})

	t.Run("flags set", func(t *testing.T) {
		const metricsAddr = "1.2.3.4:5678"

		expected := &CommandLine{
			EnableLeaderElection:        true,
			LeaderElectionLeaseDuration: 123 * time.Second,
			MetricsAddr:                 metricsAddr,
		}

		args := []string{
			"--enable-leader-election",
			"--leader-election-lease-duration", "123s",
			"--metrics-addr", metricsAddr,
		}

		cl, err := ParseCommandLine("test", args)
		assert.NoError(t, err)

		assert.Equal(t, expected, cl)
	})
}
