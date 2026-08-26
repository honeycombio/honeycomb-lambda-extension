package telemetryapi

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"math"
	"strings"
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

// A record that is valid JSON can still fail to decode into a Go value, which
// used to drop the message with only a warning in the extension's own log.
// Nothing else reports it, so the function's output would simply be missing.
func TestUndecodableRecordIsReportedNotDropped(t *testing.T) {
	// A number outside float64's range is the only way to reach this: the
	// batch itself already decoded, so the record is syntactically valid.
	const body = `[{"time":"2025-07-20T08:26:40.000Z","type":"function","record":{"n":1e999}}]`

	events := postBody(t, body, otlpConfig)

	require.Len(t, events, 1, "the message should still reach Honeycomb")
	assert.Equal(t, `{"n":1e999}`, events[0].Data["record"],
		"the record it could not decode is carried as the line it was")
}

// Detection now runs on the record's raw bytes, ahead of the decode into
// interface{} that the ordinary log path performs. That reorder is observable:
// a record that cannot decode into interface{} -- a number outside float64's
// range is the only way -- used to be reported as a raw line without ever being
// offered to the translator. An export request carrying one is now recognized.
// Pinned because it is a considered consequence of the reorder rather than an
// accident of it.
func TestUndecodableNumbersDoNotHideAnExportRequest(t *testing.T) {
	t.Run("envelope carrying one is still translated", func(t *testing.T) {
		envelope := otlpStdoutEnvelopeFixture(t)
		withJunk := strings.TrimSuffix(envelope, "}") + `,"junk":1e400}`

		events := postMessagesWithConfig(t, []LogMessage{{
			Time:   "2025-07-20T08:26:40.000Z",
			Type:   "function",
			Record: rec(withJunk),
		}}, otlpConfig)

		require.NotEmpty(t, events)
		assert.Equal(t, "my-func", events[0].Dataset,
			"the payload translates and routes by service.name")
	})

	t.Run("a bare export request carrying one does not translate", func(t *testing.T) {
		// husky rejects unknown top-level fields, so this one falls through to
		// the log path. Pinned so that a future loosening there is a visible
		// change rather than a silent one.
		events := postMessagesWithConfig(t, []LogMessage{{
			Time:   "2025-07-20T08:26:40.000Z",
			Type:   "function",
			Record: rec(strings.TrimSuffix(twoSpanExport, "}") + `,"junk":1e400}`),
		}}, otlpConfig)

		require.Len(t, events, 1)
		assert.Contains(t, events[0].Data, "record", "reported as an ordinary log line")
		assert.Equal(t, "configured-dataset", events[0].Dataset)
	})
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

// Translated telemetry is deliberately marked with the extension's own field, so
// a query can tell it apart from telemetry sent directly to Honeycomb. Pinned
// because it is a considered deviation from what the OTLP endpoint produces.
func TestTranslatedEventsCarryExtensionMetadata(t *testing.T) {
	events := postMessagesWithConfig(t, []LogMessage{{
		Time:   "2025-07-20T08:26:40.000Z",
		Type:   "function",
		Record: recString(twoSpanExport),
	}}, otlpConfig)

	require.Len(t, events, 2)
	for _, event := range events {
		assert.Equal(t, "function", event.Data["lambda_extension.type"])
		assert.Equal(t, "my-func", event.Dataset, "still routed by service.name")
	}
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

// husky does not clamp every rate it derives, so the conversion to libhoney's
// unsigned rate is where a bad value has to be stopped.
func TestSampleRateFloor(t *testing.T) {
	testCases := []struct {
		derived int32
		want    uint
	}{
		{-1, 1},
		{0, 1},
		{1, 1},
		{100, 100},
		{math.MaxInt32, math.MaxInt32},
	}

	for _, tc := range testCases {
		assert.EqualValues(t, tc.want, sampleRate(tc.derived),
			"a rate of %d must not become a weight of %d", tc.derived, uint(tc.derived))
	}
}

// The rate husky derives from a sampling threshold is the path that reaches the
// conversion unclamped: an adjusted count above int32 overflows the conversion
// husky makes, and Go leaves the result of an out-of-range float conversion to
// the architecture. On amd64 it becomes negative, which is how this reaches the
// floor above; on arm64 the same conversion saturates, so this payload alone
// does not prove the floor is needed and TestSampleRateFloor does.
func TestOTLPSampleRateFromAThresholdIsUsable(t *testing.T) {
	const thresholdSampled = `{"resourceSpans":[{"resource":{"attributes":[
		{"key":"service.name","value":{"stringValue":"my-func"}}]},
		"scopeSpans":[{"spans":[{"traceId":"5b8efff798038103d269b633813fc60c",
		"spanId":"eee19b7ec3c1b174","name":"handler","traceState":"ot=th:ffffffffffffff",
		"startTimeUnixNano":"1753000000000000000","endTimeUnixNano":"1753000000001000000"}]}]}]}`

	events := postMessagesWithConfig(t, []LogMessage{{
		Time:   "2025-07-20T08:26:40.000Z",
		Type:   "function",
		Record: recString(thresholdSampled),
	}}, otlpConfig)

	require.Len(t, events, 1)
	assert.LessOrEqual(t, events[0].SampleRate, uint(math.MaxInt32),
		"a wrapped conversion would exceed any rate husky can mean")
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

// The otlp-stdout exporters wrap a compressed payload rather than writing
// OTLP/JSON, and that has to work through the handler as well.
func TestOTLPStdoutEnvelopeThroughHandler(t *testing.T) {
	events := postMessagesWithConfig(t, []LogMessage{{
		Time:   "2025-07-20T08:26:40.000Z",
		Type:   "function",
		Record: recString(otlpStdoutEnvelopeFixture(t)),
	}}, otlpConfig)

	require.Len(t, events, 1)
	assert.Equal(t, "my-func", events[0].Dataset)
	assert.Equal(t, "handler", events[0].Data["name"])
	assert.Equal(t, "function", events[0].Data["lambda_extension.type"])
}

// otlpStdoutEnvelopeFixture is one line of gzipped, base64-encoded OTLP JSON in
// the envelope those exporters emit.
func otlpStdoutEnvelopeFixture(t *testing.T) string {
	t.Helper()
	const span = `{"resourceSpans":[{"resource":{"attributes":[
		{"key":"service.name","value":{"stringValue":"my-func"}}]},
		"scopeSpans":[{"spans":[{"traceId":"5b8efff798038103d269b633813fc60c",
		"spanId":"eee19b7ec3c1b174","name":"handler",
		"startTimeUnixNano":"1753000000000000000","endTimeUnixNano":"1753000000123000000"}]}]}]}`

	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err := writer.Write([]byte(span))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	line, err := json.Marshal(map[string]interface{}{
		"__otel_otlp_stdout": "otlp-stdout-span-exporter@0.15.0",
		"source":             "my-func",
		"endpoint":           "http://localhost:4318/v1/traces",
		"content-type":       "application/json",
		"content-encoding":   "gzip",
		"payload":            base64.StdEncoding.EncodeToString(compressed.Bytes()),
		"base64":             true,
	})
	require.NoError(t, err)
	return string(line)
}
