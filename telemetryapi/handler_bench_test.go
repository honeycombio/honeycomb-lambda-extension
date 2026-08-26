package telemetryapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/honeycombio/honeycomb-lambda-extension/extension"
	libhoney "github.com/honeycombio/libhoney-go"
	"github.com/honeycombio/libhoney-go/transmission"
)

// The handler runs once per telemetry batch in a process that shares the
// function's memory and CPU allocation, so the cost of each record shape is
// worth being able to measure. One shape's cost is paid to reduce another's, so
// a claim about any of them means nothing without the others beside it.

const benchAPIKey = "0123456789abcdef0123456789abcdef"

// benchTracesRecord is an export request of n spans, as a function batching its
// spans to stdout would write it.
func benchTracesRecord(n int) string {
	spans := make([]string, 0, n)
	for i := 0; i < n; i++ {
		spans = append(spans, fmt.Sprintf(`{"traceId":"5b8efff798038103d269b633813fc60c","spanId":"eee19b7ec3c1b1%02d","name":"span-%d","kind":2,"startTimeUnixNano":"1753000000000000000","endTimeUnixNano":"1753000000123000000","attributes":[{"key":"http.status_code","value":{"intValue":"200"}},{"key":"http.route","value":{"stringValue":"/a/b/c"}}]}`, i, i))
	}
	return `{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"my-func"}}]},"scopeSpans":[{"scope":{"name":"inst","version":"1.2.3"},"spans":[` +
		strings.Join(spans, ",") + `]}]}]}`
}

func benchmarkHandler(b *testing.B, record string) {
	b.Helper()
	body, err := json.Marshal([]LogMessage{{
		Time:   "2025-07-20T08:26:40.000Z",
		Type:   "function",
		Record: json.RawMessage(record),
	}})
	if err != nil {
		b.Fatal(err)
	}

	client, err := libhoney.NewClient(libhoney.ClientConfig{
		Transmission: &transmission.DiscardSender{},
		APIKey:       benchAPIKey,
		Dataset:      "bench",
	})
	if err != nil {
		b.Fatal(err)
	}
	serve := handler(client, extension.Config{APIKey: benchAPIKey, Dataset: "bench"})

	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		request, _ := http.NewRequest("POST", "/", bytes.NewReader(body))
		serve(httptest.NewRecorder(), request)
	}
}

func BenchmarkHandlerOTLPTraces(b *testing.B) {
	benchmarkHandler(b, benchTracesRecord(50))
}

// An OTLP logs payload whose records carry a message attribute. The scan that
// lets a traces payload skip the wrapper decode does not help here, so this is
// the case where recognizing a record costs the most.
func BenchmarkHandlerOTLPLogsWithMessageAttribute(b *testing.B) {
	records := make([]string, 0, 50)
	for i := 0; i < 50; i++ {
		records = append(records, fmt.Sprintf(`{"timeUnixNano":"1753000000000000000","severityText":"INFO","body":{"stringValue":"line %d"},"attributes":[{"key":"message","value":{"stringValue":"m%d"}}]}`, i, i))
	}
	benchmarkHandler(b, `{"resourceLogs":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"my-func"}}]},"scopeLogs":[{"logRecords":[`+
		strings.Join(records, ",")+`]}]}]}`)
}

func BenchmarkHandlerPlainLine(b *testing.B) {
	benchmarkHandler(b, `"an ordinary log line from the function"`)
}

// The JSON-log-format wrapper: the shape whose keys the scan is looking for,
// and the only one that pays for the decode it guards.
func BenchmarkHandlerWrappedLine(b *testing.B) {
	benchmarkHandler(b, `{"timestamp":"2025-07-20T08:26:40.000Z","level":"INFO","message":"an ordinary log line from the function","requestId":"6d67e385-053d-4622-a56f-b25bcef23083"}`)
}

func BenchmarkHandlerLibhoneyEnvelope(b *testing.B) {
	benchmarkHandler(b, `{"time":"2025-07-20T08:26:40.000Z","samplerate":1,"data":{"name":"s","duration_ms":12.5,"a":1,"b":"x","c":true}}`)
}
