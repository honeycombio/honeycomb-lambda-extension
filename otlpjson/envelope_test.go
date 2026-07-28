package otlpjson

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	collectorTrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	common "go.opentelemetry.io/proto/otlp/common/v1"
	resource "go.opentelemetry.io/proto/otlp/resource/v1"
	trace "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

// tracesProtobuf builds the same span the JSON fixtures carry, in the protobuf
// encoding the otlp-stdout exporters actually emit.
func tracesProtobuf(t *testing.T) []byte {
	t.Helper()
	request := &collectorTrace.ExportTraceServiceRequest{
		ResourceSpans: []*trace.ResourceSpans{{
			Resource: &resource.Resource{
				Attributes: []*common.KeyValue{{
					Key:   "service.name",
					Value: &common.AnyValue{Value: &common.AnyValue_StringValue{StringValue: "my-func"}},
				}},
			},
			ScopeSpans: []*trace.ScopeSpans{{
				Spans: []*trace.Span{{
					TraceId:           []byte{0x5b, 0x8e, 0xff, 0xf7, 0x98, 0x03, 0x81, 0x03, 0xd2, 0x69, 0xb6, 0x33, 0x81, 0x3f, 0xc6, 0x0c},
					SpanId:            []byte{0xee, 0xe1, 0x9b, 0x7e, 0xc3, 0xc1, 0xb1, 0x74},
					Name:              "handler",
					Kind:              trace.Span_SPAN_KIND_SERVER,
					StartTimeUnixNano: 1753000000000000000,
					EndTimeUnixNano:   1753000000123000000,
				}},
			}},
		}},
	}
	encoded, err := proto.Marshal(request)
	require.NoError(t, err)
	return encoded
}

func gzipped(t *testing.T, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	_, err := writer.Write(body)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return buf.Bytes()
}

// envelope builds an otlp-stdout exporter line around a payload.
func envelope(t *testing.T, fields map[string]interface{}) string {
	t.Helper()
	fields["__otel_otlp_stdout"] = "otlp-stdout-span-exporter@0.15.0"
	fields["source"] = "my-func"
	encoded, err := json.Marshal(fields)
	require.NoError(t, err)
	return string(encoded)
}

// The envelope declares its own content type and encoding, and all the
// combinations the exporters can be configured for have to survive the trip.
func TestParseEnvelopeEncodings(t *testing.T) {
	protobufBody := tracesProtobuf(t)

	testCases := []struct {
		name   string
		fields map[string]interface{}
	}{
		{
			name: "gzipped protobuf, the exporter default",
			fields: map[string]interface{}{
				"endpoint":         "http://localhost:4318/v1/traces",
				"content-type":     "application/x-protobuf",
				"content-encoding": "gzip",
				"payload":          base64.StdEncoding.EncodeToString(gzipped(t, protobufBody)),
				"base64":           true,
			},
		},
		{
			name: "uncompressed protobuf",
			fields: map[string]interface{}{
				"endpoint":     "http://localhost:4318/v1/traces",
				"content-type": "application/x-protobuf",
				"payload":      base64.StdEncoding.EncodeToString(protobufBody),
				"base64":       true,
			},
		},
		{
			name: "gzipped JSON",
			fields: map[string]interface{}{
				"endpoint":         "http://localhost:4318/v1/traces",
				"content-type":     "application/json",
				"content-encoding": "gzip",
				"payload":          base64.StdEncoding.EncodeToString(gzipped(t, []byte(tracesRecord))),
				"base64":           true,
			},
		},
		{
			name: "JSON left as plain text",
			fields: map[string]interface{}{
				"endpoint":     "http://localhost:4318/v1/traces",
				"content-type": "application/json",
				"payload":      tracesRecord,
				"base64":       false,
			},
		},
		{
			name: "endpoint omitted, defaults to traces",
			fields: map[string]interface{}{
				"content-type":     "application/x-protobuf",
				"content-encoding": "gzip",
				"payload":          base64.StdEncoding.EncodeToString(gzipped(t, protobufBody)),
				"base64":           true,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := Parse([]byte(envelope(t, tc.fields)))
			require.NoError(t, err)
			require.NotNil(t, payload)
			assert.Equal(t, SignalTraces, payload.Signal)

			batches, err := Translate(context.Background(), payload, esAPIKey, "fallback-dataset")
			require.NoError(t, err)
			require.Len(t, batches, 1)
			require.Len(t, batches[0].Events, 1)

			event := batches[0].Events[0]
			assert.Equal(t, "my-func", batches[0].Dataset)
			assert.Equal(t, "handler", event.Attributes["name"])
			assert.Equal(t, "5b8efff798038103d269b633813fc60c", event.Attributes["trace.trace_id"])
			assert.Equal(t, time.Unix(1753000000, 0).UTC(), event.Timestamp.UTC())
		})
	}
}

func TestParseEnvelopeSignalFromEndpoint(t *testing.T) {
	testCases := []struct {
		name     string
		endpoint string
		want     Signal
		wantErr  bool
	}{
		{"traces", "http://localhost:4318/v1/traces", SignalTraces, false},
		{"traces with trailing slash", "http://localhost:4318/v1/traces/", SignalTraces, false},
		{"logs", "http://localhost:4318/v1/logs", SignalLogs, false},
		{"omitted", "", SignalTraces, false},
		{"metrics is unsupported", "http://localhost:4318/v1/metrics", SignalNone, true},
		{"unrecognized path", "http://localhost:4318/v1/profiles", SignalNone, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			line := envelope(t, map[string]interface{}{
				"endpoint":     tc.endpoint,
				"content-type": "application/json",
				"payload":      tracesRecord,
				"base64":       false,
			})
			payload, err := Parse([]byte(line))
			if tc.wantErr {
				assert.Error(t, err, "an unsupported signal should be reported, not silently ignored")
				return
			}
			require.NoError(t, err)
			require.NotNil(t, payload)
			assert.Equal(t, tc.want, payload.Signal)
		})
	}
}

// A line announcing itself as an envelope but carrying nothing usable is an
// error rather than an unrecognized line, so the user learns why.
func TestParseEnvelopeErrors(t *testing.T) {
	testCases := []struct {
		name   string
		fields map[string]interface{}
	}{
		{"payload is not base64", map[string]interface{}{"payload": "not!valid!base64", "base64": true}},
		{"payload is empty", map[string]interface{}{"payload": "", "base64": true}},
		{"payload absent", map[string]interface{}{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.fields["content-type"] = "application/x-protobuf"
			_, err := Parse([]byte(envelope(t, tc.fields)))
			assert.Error(t, err)
		})
	}
}

// The marker is what distinguishes an envelope, so a line that merely has a
// payload field is ordinary log output.
func TestParseIgnoresPayloadFieldWithoutMarker(t *testing.T) {
	payload, err := Parse([]byte(`{"payload":"aGVsbG8=","base64":true,"content-type":"application/x-protobuf"}`))
	require.NoError(t, err)
	assert.Nil(t, payload)
}
