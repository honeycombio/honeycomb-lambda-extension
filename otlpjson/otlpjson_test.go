package otlpjson

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An Environments & Services key. Keys of this length let husky derive the
// destination dataset from service.name.
const esAPIKey = "abc123def456ghi789jkl012m"

// A classic key, which pins everything to the configured dataset.
const classicAPIKey = "0123456789abcdef0123456789abcdef"

const tracesRecord = `{"resourceSpans":[{"resource":{"attributes":[
	{"key":"service.name","value":{"stringValue":"my-func"}}]},
	"scopeSpans":[{"scope":{"name":"my-instrumentation","version":"1.2.3"},
	"spans":[{"traceId":"5b8efff798038103d269b633813fc60c","spanId":"eee19b7ec3c1b174",
	"parentSpanId":"eee19b7ec3c1b173","name":"handler","kind":2,
	"startTimeUnixNano":"1753000000000000000","endTimeUnixNano":"1753000000123000000",
	"attributes":[{"key":"http.status_code","value":{"intValue":"200"}}]}]}]}]}`

const logsRecord = `{"resourceLogs":[{"resource":{"attributes":[
	{"key":"service.name","value":{"stringValue":"my-func"}}]},
	"scopeLogs":[{"logRecords":[{"timeUnixNano":"1753000000000000000",
	"severityText":"ERROR","body":{"stringValue":"it broke"}}]}]}]}`

func TestDetect(t *testing.T) {
	testCases := []struct {
		name   string
		record string
		want   Signal
	}{
		{"traces camelCase", `{"resourceSpans":[]}`, SignalTraces},
		{"traces snake_case", `{"resource_spans":[]}`, SignalTraces},
		{"logs camelCase", `{"resourceLogs":[]}`, SignalLogs},
		{"logs snake_case", `{"resource_logs":[]}`, SignalLogs},
		{"full traces payload", tracesRecord, SignalTraces},
		{"full logs payload", logsRecord, SignalLogs},
		{"libhoney envelope", `{"time":"2025-07-20T08:26:40Z","data":{"name":"handler"}}`, SignalNone},
		{"flat user JSON", `{"level":"info","message":"hello"}`, SignalNone},
		{"resourceSpans nested, not top level", `{"data":{"resourceSpans":[]}}`, SignalNone},
		{"not JSON", `START RequestId: abc Version: $LATEST`, SignalNone},
		{"JSON array", `[{"resourceSpans":[]}]`, SignalNone},
		{"empty", ``, SignalNone},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Detect([]byte(tc.record)))
		})
	}
}

// Spans should translate to the same fields Honeycomb's OTLP endpoint would
// produce, so that switching transports doesn't change what a user queries on.
func TestTranslateTraces(t *testing.T) {
	batches, err := Translate(context.Background(), SignalTraces, []byte(tracesRecord), esAPIKey, "fallback-dataset")
	require.NoError(t, err)
	require.Len(t, batches, 1)

	batch := batches[0]
	assert.Equal(t, "my-func", batch.Dataset, "dataset should come from service.name for an E&S key")
	require.Len(t, batch.Events, 1)

	event := batch.Events[0]
	assert.Equal(t, time.Unix(1753000000, 0).UTC(), event.Timestamp.UTC())
	assert.EqualValues(t, 1, event.SampleRate)

	expectedAttrs := map[string]interface{}{
		"name":             "handler",
		"trace.trace_id":   "5b8efff798038103d269b633813fc60c",
		"trace.span_id":    "eee19b7ec3c1b174",
		"trace.parent_id":  "eee19b7ec3c1b173",
		"duration_ms":      float64(123),
		"span.kind":        "server",
		"service.name":     "my-func",
		"library.name":     "my-instrumentation",
		"library.version":  "1.2.3",
		"http.status_code": int64(200),
		"meta.signal_type": "trace",
	}
	for key, want := range expectedAttrs {
		assert.Equal(t, want, event.Attributes[key], "attribute %q", key)
	}
}

func TestTranslateLogs(t *testing.T) {
	batches, err := Translate(context.Background(), SignalLogs, []byte(logsRecord), esAPIKey, "fallback-dataset")
	require.NoError(t, err)
	require.Len(t, batches, 1)
	require.Len(t, batches[0].Events, 1)

	event := batches[0].Events[0]
	assert.Equal(t, time.Unix(1753000000, 0).UTC(), event.Timestamp.UTC())
	assert.Equal(t, "it broke", event.Attributes["body"])
	assert.Equal(t, "ERROR", event.Attributes["severity_text"])
	assert.Equal(t, "log", event.Attributes["meta.signal_type"])
}

// Classic keys have no notion of a service-derived dataset, so everything lands
// in the dataset the extension is configured with.
func TestTranslateClassicKeyUsesConfiguredDataset(t *testing.T) {
	batches, err := Translate(context.Background(), SignalTraces, []byte(tracesRecord), classicAPIKey, "fallback-dataset")
	require.NoError(t, err)
	require.Len(t, batches, 1)
	assert.Equal(t, "fallback-dataset", batches[0].Dataset)
}

func TestTranslateErrors(t *testing.T) {
	testCases := []struct {
		name    string
		signal  Signal
		record  string
		apiKey  string
		dataset string
	}{
		{"malformed OTLP", SignalTraces, `{"resourceSpans":"not an array"}`, esAPIKey, "ds"},
		{"signal mismatch", SignalLogs, tracesRecord, esAPIKey, "ds"},
		{"no signal", SignalNone, tracesRecord, esAPIKey, "ds"},
		{"missing API key", SignalTraces, tracesRecord, "", "ds"},
		{"classic key without dataset", SignalTraces, tracesRecord, classicAPIKey, ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Translate(context.Background(), tc.signal, []byte(tc.record), tc.apiKey, tc.dataset)
			assert.Error(t, err)
		})
	}
}
