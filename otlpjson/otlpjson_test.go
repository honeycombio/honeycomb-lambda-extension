package otlpjson

import (
	"context"
	"testing"
	"time"

	"github.com/honeycombio/husky/otlp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// husky treats a key as classic only if it is 32 characters of lowercase hex;
// anything else routes by service.name.
const esAPIKey = "abc123def456ghi789jkl012m"
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

func TestParseSignals(t *testing.T) {
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
			payload, err := Parse([]byte(tc.record))
			require.NoError(t, err)
			assert.Equal(t, tc.want, signalOf(payload))
		})
	}
}

// Spans should translate to the same fields Honeycomb's OTLP endpoint would
// produce, so that switching transports doesn't change what a user queries on.
func TestTranslateTraces(t *testing.T) {
	batches, err := Translate(context.Background(), mustParse(t, tracesRecord), esAPIKey, "fallback-dataset")
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
	batches, err := Translate(context.Background(), mustParse(t, logsRecord), esAPIKey, "fallback-dataset")
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
	batches, err := Translate(context.Background(), mustParse(t, tracesRecord), classicAPIKey, "fallback-dataset")
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
		wantErr error
	}{
		{"malformed OTLP", SignalTraces, `{"resourceSpans":"not an array"}`, esAPIKey, "ds", otlp.ErrFailedParseBody},
		{"signal mismatch", SignalLogs, tracesRecord, esAPIKey, "ds", otlp.ErrFailedParseBody},
		{"missing API key", SignalTraces, tracesRecord, "", "ds", otlp.ErrMissingAPIKeyHeader},
		{"classic key without dataset", SignalTraces, tracesRecord, classicAPIKey, "", otlp.ErrMissingDatasetHeader},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payload := &Payload{Signal: tc.signal, Body: []byte(tc.record), ContentType: "application/json"}
			_, err := Translate(context.Background(), payload, tc.apiKey, tc.dataset)
			assert.ErrorIs(t, err, tc.wantErr)
		})
	}

	t.Run("no signal", func(t *testing.T) {
		payload := &Payload{Signal: SignalNone, Body: []byte(tracesRecord), ContentType: "application/json"}
		_, err := Translate(context.Background(), payload, esAPIKey, "ds")
		assert.Error(t, err, "callers must not ask for a translation of a non-OTLP payload")
	})
}

// A record carrying both signals is malformed. Traces win; pinned here so the
// choice is visible if it ever changes.
func TestParsePrefersTracesWhenBothPresent(t *testing.T) {
	payload, err := Parse([]byte(`{"resourceSpans":[],"resourceLogs":[]}`))
	require.NoError(t, err)
	assert.Equal(t, SignalTraces, signalOf(payload))
}

// husky groups events by dataset, so one line naming two services must produce
// two batches rather than collapsing into one.
func TestTranslateSplitsResourcesByService(t *testing.T) {
	const twoServices = `{"resourceSpans":[
		{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"svc-a"}}]},
		"scopeSpans":[{"spans":[{"traceId":"5b8efff798038103d269b633813fc60c",
		"spanId":"eee19b7ec3c1b174","name":"a","startTimeUnixNano":"1753000000000000000",
		"endTimeUnixNano":"1753000000001000000"}]}]},
		{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"svc-b"}}]},
		"scopeSpans":[{"spans":[{"traceId":"5b8efff798038103d269b633813fc60c",
		"spanId":"eee19b7ec3c1b175","name":"b","startTimeUnixNano":"1753000000000000000",
		"endTimeUnixNano":"1753000000001000000"}]}]}]}`

	batches, err := Translate(context.Background(), mustParse(t, twoServices), esAPIKey, "fallback-dataset")
	require.NoError(t, err)

	datasets := make(map[string]int)
	for _, batch := range batches {
		datasets[batch.Dataset] += len(batch.Events)
	}
	assert.Equal(t, map[string]int{"svc-a": 1, "svc-b": 1}, datasets)
}

// snake_case is legal OTLP JSON, and husky has to translate it, not just pass
// Detect.
func TestTranslateSnakeCase(t *testing.T) {
	const snakeCase = `{"resource_spans":[{"resource":{"attributes":[
		{"key":"service.name","value":{"string_value":"my-func"}}]},
		"scope_spans":[{"spans":[{"trace_id":"5b8efff798038103d269b633813fc60c",
		"span_id":"eee19b7ec3c1b174","name":"handler",
		"start_time_unix_nano":"1753000000000000000",
		"end_time_unix_nano":"1753000000123000000"}]}]}]}`

	batches, err := Translate(context.Background(), mustParse(t, snakeCase), esAPIKey, "fallback-dataset")
	require.NoError(t, err)
	require.Len(t, batches, 1)
	require.Len(t, batches[0].Events, 1)
	assert.Equal(t, "my-func", batches[0].Dataset)
	assert.Equal(t, "handler", batches[0].Events[0].Attributes["name"])
	assert.EqualValues(t, 123, batches[0].Events[0].Attributes["duration_ms"])
}

// mustParse fails the test if a fixture the test treats as OTLP isn't recognized.
func mustParse(t *testing.T, record string) *Payload {
	t.Helper()
	payload, err := Parse([]byte(record))
	require.NoError(t, err)
	require.NotNil(t, payload, "fixture should be recognized as an export request")
	return payload
}

func signalOf(payload *Payload) Signal {
	if payload == nil {
		return SignalNone
	}
	return payload.Signal
}
