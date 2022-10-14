package main

import (
	"testing"
	"time"

	"github.com/honeycombio/honeycomb-lambda-extension/eventpublisher"
	"github.com/stretchr/testify/assert"
)

func Test_Configuration_Timeouts(t *testing.T) {
	testCases := []struct {
		desc                   string
		timeoutEnvVar          string
		expectedBatchTimeout   time.Duration
		expectedConnectTimeout time.Duration
	}{
		{
			desc:                   "default",
			timeoutEnvVar:          "",
			expectedBatchTimeout:   eventpublisher.DefaultBatchSendTimeout,
			expectedConnectTimeout: eventpublisher.DefaultConnectTimeout,
		},
		{
			desc:                   "set by user: duration seconds",
			timeoutEnvVar:          "9s",
			expectedBatchTimeout:   9 * time.Second,
			expectedConnectTimeout: 9 * time.Second,
		},
		{
			desc:                   "set by user: duration milliseconds",
			timeoutEnvVar:          "900ms",
			expectedBatchTimeout:   900 * time.Millisecond,
			expectedConnectTimeout: 900 * time.Millisecond,
		},
		{
			desc:                   "set by user: integer",
			timeoutEnvVar:          "42",
			expectedBatchTimeout:   42 * time.Second,
			expectedConnectTimeout: 42 * time.Second,
		},
		{
			desc:                   "bad input: words",
			timeoutEnvVar:          "forty-two",
			expectedBatchTimeout:   eventpublisher.DefaultBatchSendTimeout,
			expectedConnectTimeout: eventpublisher.DefaultConnectTimeout,
		},
		{
			desc:                   "bad input: unicode",
			timeoutEnvVar:          "🤷",
			expectedBatchTimeout:   eventpublisher.DefaultBatchSendTimeout,
			expectedConnectTimeout: eventpublisher.DefaultConnectTimeout,
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			t.Setenv("HONEYCOMB_BATCH_SEND_TIMEOUT", tC.timeoutEnvVar)
			t.Setenv("HONEYCOMB_CONNECT_TIMEOUT", tC.timeoutEnvVar)
			assert.Equal(t, tC.expectedBatchTimeout, envOrElseDuration("HONEYCOMB_BATCH_SEND_TIMEOUT",
				eventpublisher.DefaultBatchSendTimeout))
			assert.Equal(t, tC.expectedConnectTimeout, envOrElseDuration("HONEYCOMB_CONNECT_TIMEOUT",
				eventpublisher.DefaultConnectTimeout))
		})
	}
}
