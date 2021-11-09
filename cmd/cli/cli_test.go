package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseCommandLine(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		cl, err := ParseCommandLine("test", nil)
		assert.NoError(t, err)

		assert.False(t, cl.EnableLeaderElection)
		assert.Equal(t, ":8080", cl.MetricsAddr)
	})

	t.Run("flags set", func(t *testing.T) {
		const metricsAddr = "1.2.3.4:5678"

		expected := &CommandLine{
			EnableLeaderElection: true,
			MetricsAddr:          metricsAddr,
		}

		args := []string{
			"--enable-leader-election",
			"--metrics-addr", metricsAddr,
		}

		cl, err := ParseCommandLine("test", args)
		assert.NoError(t, err)

		assert.Equal(t, expected, cl)
	})
}
