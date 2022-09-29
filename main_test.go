package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_Configuration_BatchSendTimeout(t *testing.T) {
	testCases := []struct {
		desc            string
		timeoutEnvVar   string
		expectedTimeout time.Duration
	}{
		{
			desc:            "default",
			timeoutEnvVar:   "",
			expectedTimeout: defaultBatchSendTimeout,
		},
		{
			desc:            "set by user",
			timeoutEnvVar:   "900",
			expectedTimeout: 900 * time.Second,
		},
		{
			desc:            "bad input",
			timeoutEnvVar:   "🤷‍♂️",
			expectedTimeout: defaultBatchSendTimeout,
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			t.Setenv("HONEYCOMB_BATCH_SEND_TIMEOUT_S", tC.timeoutEnvVar)
			assert.Equal(t, tC.expectedTimeout, newTransmission().BatchSendTimeout)
		})
	}
}
