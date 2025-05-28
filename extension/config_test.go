package extension

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/service/kms"
	"github.com/stretchr/testify/assert"
)

func Test_EnvOrElseInt(t *testing.T) {
	aDefaultInt := 42
	testCases := []struct {
		desc          string
		envValue      string
		expectedValue int
	}{
		{
			desc:          "default",
			envValue:      "not-set",
			expectedValue: aDefaultInt,
		},
		{
			desc:          "set by user: integer",
			envValue:      "23",
			expectedValue: 23,
		},
		{
			desc:          "bad input: words",
			envValue:      "twenty-three",
			expectedValue: aDefaultInt,
		},
		{
			desc:          "bad input: unicode",
			envValue:      "🤷",
			expectedValue: aDefaultInt,
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			if tC.envValue != "not-set" {
				t.Setenv("SOME_TEST_ENV_VAR", tC.envValue)
			}
			assert.Equal(t, tC.expectedValue, envOrElseInt("SOME_TEST_ENV_VAR", aDefaultInt))
		})
	}
}

func Test_EnvOrElseBool(t *testing.T) {
	aDefaultBool := false
	testCases := []struct {
		desc          string
		envValue      string
		expectedValue bool
	}{
		{
			desc:          "default",
			envValue:      "not-set",
			expectedValue: aDefaultBool,
		},
		{
			desc:          "set by user: true",
			envValue:      "true",
			expectedValue: true,
		},
		{
			desc:          "set by user: false",
			envValue:      "false",
			expectedValue: false,
		},
		{
			desc:          "bad input: non-bool words",
			envValue:      "verily yes",
			expectedValue: aDefaultBool,
		},
		{
			desc:          "bad input: unicode",
			envValue:      "🤷",
			expectedValue: aDefaultBool,
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			if tC.envValue != "not-set" {
				t.Setenv("SOME_TEST_ENV_VAR", tC.envValue)
			}
			assert.Equal(t, tC.expectedValue, envOrElseBool("SOME_TEST_ENV_VAR", aDefaultBool))
		})
	}
}

func Test_EnvOrElseDuration(t *testing.T) {
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
				t.Setenv("SOME_TEST_ENV_VAR", tC.envValue)
			}
			assert.Equal(t, tC.expectedValue, envOrElseDuration("SOME_TEST_ENV_VAR", aDefaultDuration))
		})
	}
}

func Test_GetApiKey(t *testing.T) {
	originalApiKey := "test-api-key"
	encodedApiKey := base64.StdEncoding.EncodeToString([]byte(originalApiKey))
	assert.Equal(t, "dGVzdC1hcGkta2V5", encodedApiKey)

	originalKmsDecryptFunc := kmsDecryptFunc
	defer func() {
		kmsDecryptFunc = originalKmsDecryptFunc
	}()

	testCases := []struct {
		desc          string
		envSetup      func(t *testing.T)
		mockSetup     func(t *testing.T)
		expectedValue string
		expectError   bool
	}{
		{
			desc: "regular libhoney api key",
			envSetup: func(t *testing.T) {
				t.Setenv("LIBHONEY_API_KEY", originalApiKey)
				t.Setenv("KMS_KEY_ID", "")
			},
			expectedValue: originalApiKey,
			expectError:   false,
		},
		{
			desc: "kms-encrypted api key",
			envSetup: func(t *testing.T) {
				t.Setenv("LIBHONEY_API_KEY", encodedApiKey)
				t.Setenv("KMS_KEY_ID", "some-key-id")
				t.Setenv("AWS_REGION", "us-east-2")
			},
			mockSetup: func(t *testing.T) {
				kmsDecryptFunc = func(svc *kms.KMS, input *kms.DecryptInput) (*kms.DecryptOutput, error) {
					return &kms.DecryptOutput{
						Plaintext: []byte(originalApiKey),
					}, nil
				}
			},
			expectedValue: originalApiKey,
			expectError:   false,
		},
		{
			desc: "invalid base64 in encrypted api key",
			envSetup: func(t *testing.T) {
				t.Setenv("LIBHONEY_API_KEY", "not-valid-base64")
				t.Setenv("KMS_KEY_ID", "some-key-id")
				t.Setenv("AWS_REGION", "us-west-2")
			},
			expectError: true,
		},
		{
			desc: "KMS_KEY_ID set but not LIBHONEY_API_KEY",
			envSetup: func(t *testing.T) {
				t.Setenv("LIBHONEY_API_KEY", "")
				t.Setenv("KMS_KEY_ID", "some-key-id")
				t.Setenv("AWS_REGION", "us-west-2")
			},
			expectError: true,
		},
		{
			desc: "neither LIBHONEY_API_KEY or KMS_KEY_ID set",
			envSetup: func(t *testing.T) {
				t.Setenv("LIBHONEY_API_KEY", "")
				t.Setenv("KMS_KEY_ID", "")
			},
			expectError: true,
		},
	}

	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			tC.envSetup(t)
			if tC.mockSetup != nil {
				tC.mockSetup(t)
			}
			apiKey := getApiKey()
			if tC.expectError {
				assert.Empty(t, apiKey, "Expected empty API key due to error condition")
			} else {
				assert.Equal(t, tC.expectedValue, apiKey)
			}
		})
	}
}
