package otlpjson

import (
	"fmt"
	"strings"
	"testing"
)

// Parse runs on every line a function writes to stdout, so both the payload it
// is looking for and the ordinary log output it has to reject quickly are worth
// measuring. The comments in this package claim recognition costs a scan rather
// than a copy of the export request; these are what make that checkable.

// tracesWithSpans builds an export request of n spans, the shape and size a
// function batching its spans to stdout actually emits.
func tracesWithSpans(n int) []byte {
	spans := make([]string, 0, n)
	for i := 0; i < n; i++ {
		spans = append(spans, fmt.Sprintf(`{"traceId":"5b8efff798038103d269b633813fc60c","spanId":"eee19b7ec3c1b1%02d","name":"span-%d","kind":2,"startTimeUnixNano":"1753000000000000000","endTimeUnixNano":"1753000000123000000","attributes":[{"key":"http.status_code","value":{"intValue":"200"}},{"key":"http.route","value":{"stringValue":"/a/b/c"}}]}`, i, i))
	}
	return []byte(`{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"my-func"}}]},"scopeSpans":[{"scope":{"name":"inst","version":"1.2.3"},"spans":[` +
		strings.Join(spans, ",") + `]}]}]}`)
}

func BenchmarkParseTraces(b *testing.B) {
	payload := tracesWithSpans(50)
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	for i := 0; i < b.N; i++ {
		got, err := Parse(payload)
		if err != nil || got == nil {
			b.Fatalf("Parse: %v", err)
		}
	}
}

// The majority case: a line that is not an export request at all and has to be
// rejected before anything expensive happens.
func BenchmarkParsePlainLogLine(b *testing.B) {
	line := []byte(`"2025-07-20T08:26:40.000Z INFO handling request id=abc123 duration=12ms"`)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if got, _ := Parse(line); got != nil {
			b.Fatal("a plain log line should not parse as an export request")
		}
	}
}

// A structured log line the function emitted itself, which reaches the same
// rejection by a different route than a bare string does.
func BenchmarkParseStructuredLogLine(b *testing.B) {
	line := []byte(`{"level":"info","msg":"handled","request_id":"abc123","duration_ms":12.5}`)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if got, _ := Parse(line); got != nil {
			b.Fatal("a structured log line should not parse as an export request")
		}
	}
}
