package telemetryapi

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/honeycombio/honeycomb-lambda-extension/extension"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// otlpConfig is what the extension is configured with when a function exports
// OTLP to stdout. The key is an Environments & Services key, so the destination
// dataset comes from service.name rather than from LIBHONEY_DATASET.
var otlpConfig = extension.Config{
	APIKey:  "abc123def456ghi789jkl012m",
	Dataset: "configured-dataset",
}

// twoSpanExport is a single line of OTLP/JSON holding two spans, as an OTel SDK
// configured to export to stdout would write it.
const twoSpanExport = `{"resourceSpans":[{"resource":{"attributes":[
	{"key":"service.name","value":{"stringValue":"my-func"}}]},
	"scopeSpans":[{"scope":{"name":"my-instrumentation"},"spans":[
	{"traceId":"5b8efff798038103d269b633813fc60c","spanId":"eee19b7ec3c1b174",
	"name":"handler","kind":2,"startTimeUnixNano":"1753000000000000000",
	"endTimeUnixNano":"1753000000123000000"},
	{"traceId":"5b8efff798038103d269b633813fc60c","spanId":"eee19b7ec3c1b175",
	"parentSpanId":"eee19b7ec3c1b174","name":"db.query","kind":3,
	"startTimeUnixNano":"1753000000010000000","endTimeUnixNano":"1753000000020000000"}]}]}]}`

// An OTLP payload reaches the extension in one of two shapes depending on the
// function's log format, and must produce the same events either way.
func TestOTLPTracesInBothLogFormats(t *testing.T) {
	testCases := []struct {
		name   string
		record json.RawMessage
	}{
		{"plain-text log format, record is a JSON string", recString(twoSpanExport)},
		{"JSON log format, record is pre-parsed", rec(twoSpanExport)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			events := postMessagesWithConfig(t, []LogMessage{{
				Time:   "2025-07-20T08:26:40.000Z",
				Type:   "function",
				Record: tc.record,
			}}, otlpConfig)

			require.Len(t, events, 2, "each span in the export should become an event")

			for _, event := range events {
				assert.Equal(t, "my-func", event.Dataset, "dataset should come from service.name")
				assert.Equal(t, "5b8efff798038103d269b633813fc60c", event.Data["trace.trace_id"])
				assert.Equal(t, "function", event.Data["lambda_extension.type"])
			}

			assert.Equal(t, "handler", events[0].Data["name"])
			assert.Equal(t, "db.query", events[1].Data["name"])
			assert.Equal(t, "eee19b7ec3c1b174", events[1].Data["trace.parent_id"])
			assert.Equal(t, time.Unix(1753000000, 0).UTC(), events[0].Timestamp.UTC())
		})
	}
}

func TestOTLPLogs(t *testing.T) {
	const logExport = `{"resourceLogs":[{"resource":{"attributes":[
		{"key":"service.name","value":{"stringValue":"my-func"}}]},
		"scopeLogs":[{"logRecords":[{"timeUnixNano":"1753000000000000000",
		"severityText":"ERROR","body":{"stringValue":"it broke"}}]}]}]}`

	events := postMessagesWithConfig(t, []LogMessage{{
		Time:   "2025-07-20T08:26:40.000Z",
		Type:   "function",
		Record: recString(logExport),
	}}, otlpConfig)

	require.Len(t, events, 1)
	assert.Equal(t, "my-func", events[0].Dataset)
	assert.Equal(t, "it broke", events[0].Data["body"])
	assert.Equal(t, "ERROR", events[0].Data["severity_text"])
}

// Nanosecond timestamps written as JSON numbers exceed what a float64 can hold
// exactly, so the record's original bytes have to reach the translator. This is
// the case that decoding the record to interface{} first would silently corrupt.
func TestOTLPNumericNanosecondsKeepFullPrecision(t *testing.T) {
	const preciseExport = `{"resourceSpans":[{"resource":{"attributes":[
		{"key":"service.name","value":{"stringValue":"my-func"}}]},
		"scopeSpans":[{"spans":[{"traceId":"5b8efff798038103d269b633813fc60c",
		"spanId":"eee19b7ec3c1b174","name":"handler",
		"startTimeUnixNano":1753000000123456789,
		"endTimeUnixNano":1753000000223456789}]}]}]}`

	events := postMessagesWithConfig(t, []LogMessage{{
		Time:   "2025-07-20T08:26:40.000Z",
		Type:   "function",
		Record: rec(preciseExport),
	}}, otlpConfig)

	require.Len(t, events, 1)
	assert.Equal(t, time.Unix(1753000000, 123456789).UTC(), events[0].Timestamp.UTC())
}

// Anything that isn't a well-formed OTLP export from the function still has to
// reach Honeycomb the way it always has.
func TestNonOTLPRecordsAreUnaffected(t *testing.T) {
	testCases := []struct {
		name   string
		msg    LogMessage
		assert func(t *testing.T, data map[string]interface{})
	}{
		{
			name: "libhoney envelope",
			msg: LogMessage{
				Time:   "2025-07-20T08:26:40.000Z",
				Type:   "function",
				Record: recString(`{"time":"2025-07-20T08:26:40.000Z","data":{"name":"handler"}}`),
			},
			assert: func(t *testing.T, data map[string]interface{}) {
				assert.Equal(t, "handler", data["name"])
			},
		},
		{
			name: "malformed OTLP falls through to ordinary handling",
			msg: LogMessage{
				Time:   "2025-07-20T08:26:40.000Z",
				Type:   "function",
				Record: recString(`{"resourceSpans":"not an array"}`),
			},
			assert: func(t *testing.T, data map[string]interface{}) {
				assert.Equal(t, "not an array", data["resourceSpans"],
					"the unparseable payload should still be visible to the user")
			},
		},
		{
			name: "platform record is never treated as OTLP",
			msg: LogMessage{
				Time:   "2025-07-20T08:26:40.000Z",
				Type:   "platform.report",
				Record: rec(twoSpanExport),
			},
			assert: func(t *testing.T, data map[string]interface{}) {
				assert.Equal(t, "platform.report", data["lambda_extension.type"])
				assert.NotContains(t, data, "trace.trace_id")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			events := postMessagesWithConfig(t, []LogMessage{tc.msg}, otlpConfig)
			require.Len(t, events, 1)
			assert.Equal(t, "configured-dataset", events[0].Dataset,
				"only OTLP events route away from the configured dataset")
			tc.assert(t, events[0].Data)
		})
	}
}

// libhoney panics if handed a nil value to Add, so record shapes that decode to
// nothing useful have to be handled before they reach it. A panic here would
// cost the rest of the batch, since net/http only recovers at the request.
func TestUnusualRecordShapesDoNotPanic(t *testing.T) {
	testCases := []struct {
		name string
		body string
	}{
		{"record is null", `[{"time":"2025-07-20T08:26:40.000Z","type":"function","record":null}]`},
		{"record is absent", `[{"time":"2025-07-20T08:26:40.000Z","type":"function"}]`},
		{"record is a number", `[{"time":"2025-07-20T08:26:40.000Z","type":"function","record":42}]`},
		{"record is an array", `[{"time":"2025-07-20T08:26:40.000Z","type":"function","record":["a"]}]`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			events := postBody(t, tc.body, otlpConfig)
			require.Len(t, events, 1, "the message should still reach Honeycomb")
			assert.Equal(t, "function", events[0].Data["lambda_extension.type"])
		})
	}
}

// A panic in one message must not take the rest of the batch with it.
func TestUnusualRecordDoesNotLoseTheRestOfTheBatch(t *testing.T) {
	events := postBody(t, `[
		{"time":"2025-07-20T08:26:40.000Z","type":"function","record":null},
		{"time":"2025-07-20T08:26:40.000Z","type":"function","record":"a real log line"}
	]`, otlpConfig)

	require.Len(t, events, 2)
	assert.Equal(t, "a real log line", events[1].Data["record"])
}

// The sample rate husky derives has to survive onto the event, or presampled
// spans would be counted once each instead of at their true weight.
func TestOTLPSampleRateReachesTheEvent(t *testing.T) {
	const sampled = `{"resourceSpans":[{"resource":{"attributes":[
		{"key":"service.name","value":{"stringValue":"my-func"}}]},
		"scopeSpans":[{"spans":[{"traceId":"5b8efff798038103d269b633813fc60c",
		"spanId":"eee19b7ec3c1b174","name":"handler",
		"startTimeUnixNano":"1753000000000000000","endTimeUnixNano":"1753000000001000000",
		"attributes":[{"key":"sampleRate","value":{"intValue":"100"}}]}]}]}]}`

	events := postMessagesWithConfig(t, []LogMessage{{
		Time:   "2025-07-20T08:26:40.000Z",
		Type:   "function",
		Record: recString(sampled),
	}}, otlpConfig)

	require.Len(t, events, 1)
	assert.EqualValues(t, 100, events[0].SampleRate)
	assert.NotContains(t, events[0].Data, "sampleRate", "husky consumes the attribute")
}

// An export request that translates to nothing must not disappear: it falls
// through to ordinary handling so the payload stays visible.
func TestOTLPWithNoSpansFallsThrough(t *testing.T) {
	testCases := []struct {
		name   string
		record string
	}{
		{"empty resourceSpans", `{"resourceSpans":[]}`},
		{"null resourceSpans", `{"resourceSpans":null}`},
		{"resource with no spans", `{"resourceSpans":[{"scopeSpans":[]}]}`},
		{"empty resourceLogs", `{"resourceLogs":[]}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			events := postMessagesWithConfig(t, []LogMessage{{
				Time:   "2025-07-20T08:26:40.000Z",
				Type:   "function",
				Record: recString(tc.record),
			}}, otlpConfig)

			require.Len(t, events, 1, "the record must still produce an event")
			assert.Equal(t, "configured-dataset", events[0].Dataset)
		})
	}
}

// husky routes trace and log datasets differently for classic keys: traces
// honor the configured dataset, logs still follow service.name. Pinned here
// because it is asymmetric and surprising.
func TestOTLPClassicKeyRoutesLogsByServiceName(t *testing.T) {
	classicConfig := extension.Config{
		APIKey:  "0123456789abcdef0123456789abcdef",
		Dataset: "configured-dataset",
	}

	logEvents := postMessagesWithConfig(t, []LogMessage{{
		Time: "2025-07-20T08:26:40.000Z",
		Type: "function",
		Record: recString(`{"resourceLogs":[{"resource":{"attributes":[
			{"key":"service.name","value":{"stringValue":"my-func"}}]},
			"scopeLogs":[{"logRecords":[{"timeUnixNano":"1753000000000000000",
			"body":{"stringValue":"it broke"}}]}]}]}`),
	}}, classicConfig)
	require.Len(t, logEvents, 1)
	assert.Equal(t, "my-func", logEvents[0].Dataset)

	spanEvents := postMessagesWithConfig(t, []LogMessage{{
		Time:   "2025-07-20T08:26:40.000Z",
		Type:   "function",
		Record: recString(twoSpanExport),
	}}, classicConfig)
	require.Len(t, spanEvents, 2)
	assert.Equal(t, "configured-dataset", spanEvents[0].Dataset)
}
