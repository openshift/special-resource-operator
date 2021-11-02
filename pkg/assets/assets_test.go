package assets

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestValidStateName(t *testing.T) {
	cases := []struct {
		input string
		valid bool
	}{
		{
			input: "0000_test.yaml",
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

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			assert.Equal(t, c.valid, ValidStateName(c.input))
		})
	}
}
