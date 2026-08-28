package telemetryapi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/honeycombio/honeycomb-lambda-extension/extension"
	"github.com/honeycombio/libhoney-go/transmission"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The files replayed here were captured from a real Lambda function by
// testdata/capture/capture.sh, which deploys a throwaway function and records
// exactly what the Telemetry API delivers. They are the only description of the
// wire format in this repo that isn't written by hand, so they are what proves
// the handler agrees with the platform rather than with our assumptions.
//
// Both of Lambda's log formats are captured. A function emitting identical stdout
// under each must produce identical events, which is the property these assert.
const (
	textFormatGolden = "telemetry-api-text-log-format.json"
	jsonFormatGolden = "telemetry-api-json-log-format.json"
)

func replayGolden(t *testing.T, name string) []*transmission.Event {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err, "run testdata/capture/capture.sh to regenerate")

	return postBody(t, string(body), extension.Config{
		APIKey:  "abc123def456ghi789jkl012m",
		Dataset: "configured-dataset",
	})
}

// eventsByName indexes replayed events by their name field, which is how a span
// is identified once translated.
func eventsByName(events []*transmission.Event) map[string]*transmission.Event {
	byName := make(map[string]*transmission.Event)
	for _, event := range events {
		if name, ok := event.Data["name"].(string); ok {
			byName[name] = event
		}
	}
	return byName
}

func TestReplayCapturedTelemetry(t *testing.T) {
	for _, golden := range []string{textFormatGolden, jsonFormatGolden} {
		t.Run(golden, func(t *testing.T) {
			events := replayGolden(t, golden)
			byName := eventsByName(events)

			t.Run("OTLP/JSON traces become a span", func(t *testing.T) {
				event := byName["handler"]
				require.NotNil(t, event, "the OTLP/JSON span should have been translated")
				assert.Equal(t, "capture-func", event.Dataset)
				assert.Equal(t, "5b8efff798038103d269b633813fc60c", event.Data["trace.trace_id"])
				assert.Equal(t, "server", event.Data["span.kind"])
				assert.EqualValues(t, 123, event.Data["duration_ms"])
				assert.EqualValues(t, 200, event.Data["http.status_code"])
				assert.Equal(t, "capture-instrumentation", event.Data["library.name"])
			})

			t.Run("OTLP/JSON logs become a log event", func(t *testing.T) {
				var found *transmission.Event
				for _, event := range events {
					if event.Data["body"] == "captured log record" {
						found = event
					}
				}
				require.NotNil(t, found, "the OTLP/JSON log record should have been translated")
				assert.Equal(t, "capture-func", found.Dataset)
				assert.Equal(t, "ERROR", found.Data["severity_text"])
			})

			t.Run("the beeline envelope still works", func(t *testing.T) {
				event := byName["beeline-span"]
				require.NotNil(t, event, "a libhoney envelope must keep being unwrapped")
				assert.Equal(t, "configured-dataset", event.Dataset)
				assert.EqualValues(t, 12.5, event.Data["duration_ms"])
				assert.NotContains(t, event.Data, "data", "the envelope should be unwrapped, not nested")
			})

			t.Run("an ordinary log line is still a log line", func(t *testing.T) {
				var found bool
				for _, event := range events {
					if record, ok := event.Data["record"].(string); ok && record == "an ordinary log line\n" {
						found = true
						assert.Equal(t, "configured-dataset", event.Dataset)
					}
				}
				assert.True(t, found, "plain stdout should arrive as a record field")
			})

			t.Run("platform telemetry is still forwarded", func(t *testing.T) {
				seen := make(map[string]bool)
				for _, event := range events {
					if kind, ok := event.Data["lambda_extension.type"].(string); ok {
						seen[kind] = true
					}
				}
				for _, kind := range []string{"platform.start", "platform.report", "platform.initStart"} {
					assert.True(t, seen[kind], "expected %s to be forwarded", kind)
				}
			})
		})
	}
}

// The point of supporting both log formats is that they are indistinguishable
// downstream. Identical stdout under each must translate to the same events.
func TestBothLogFormatsAgree(t *testing.T) {
	textEvents := eventsByName(replayGolden(t, textFormatGolden))
	jsonEvents := eventsByName(replayGolden(t, jsonFormatGolden))

	for _, name := range []string{"handler", "beeline-span"} {
		text, json := textEvents[name], jsonEvents[name]
		require.NotNil(t, text, "%s missing from the text-format capture", name)
		require.NotNil(t, json, "%s missing from the JSON-format capture", name)

		assert.Equal(t, text.Dataset, json.Dataset, "%s: dataset differs by log format", name)
		assert.Equal(t, text.Data, json.Data, "%s: fields differ by log format", name)
		assert.Equal(t, text.Timestamp, json.Timestamp, "%s: timestamp differs by log format", name)
	}
}

// The envelope's payload is gzipped protobuf or JSON that no Lambda log format
// can reinterpret, so it must survive both byte for byte.
func TestEnvelopeSurvivesBothLogFormats(t *testing.T) {
	for _, golden := range []string{textFormatGolden, jsonFormatGolden} {
		t.Run(golden, func(t *testing.T) {
			events := replayGolden(t, golden)

			spans := 0
			for _, event := range events {
				if event.Data["name"] == "handler" && event.Dataset == "capture-func" {
					spans++
				}
			}
			// One from the OTLP/JSON line, one from the envelope's payload.
			assert.Equal(t, 2, spans, "both the direct and enveloped spans should translate")
		})
	}
}
