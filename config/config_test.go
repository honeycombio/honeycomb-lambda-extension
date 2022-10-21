package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_envOrElseDuration(t *testing.T) {
	aDefaultDuration := 42 * time.Second
	testCases := []struct {
		desc          string
		envValue      string
		expectedValue time.Duration
	}{
		{
			desc:          "default",
			envValue:      "not-set",
			expectedValue: aDefaultDuration,
		},
		{
			desc:          "set by user: duration seconds",
			envValue:      "9s",
			expectedValue: 9 * time.Second,
		},
		{
			desc:          "set by user: duration milliseconds",
			envValue:      "900ms",
			expectedValue: 900 * time.Millisecond,
		},
		{
			desc:          "set by user: duration zero",
			envValue:      "0s",
			expectedValue: aDefaultDuration,
		},
		{
			desc:          "set by user: integer",
			envValue:      "23",
			expectedValue: 23 * time.Second,
		},
		{
			desc:          "set by user: integer zero",
			envValue:      "0",
			expectedValue: aDefaultDuration,
		},
		{
			desc:          "bad input: words",
			envValue:      "forty-two",
			expectedValue: aDefaultDuration,
		},
		{
			desc:          "bad input: unicode",
			envValue:      "🤷",
			expectedValue: aDefaultDuration,
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			if tC.envValue != "not-set" {
				t.Setenv("HONEYCOMB_BATCH_SEND_TIMEOUT", tC.envValue)
			}
			assert.Equal(t, tC.expectedValue, envOrElseDuration("HONEYCOMB_BATCH_SEND_TIMEOUT", aDefaultDuration))
		})
	}
}
