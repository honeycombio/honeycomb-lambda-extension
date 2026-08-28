// Command handler is a throwaway Lambda function whose only job is to write, to
// real stdout in a real execution environment, one line of every shape the
// extension is expected to recognize. Whatever Lambda then delivers to the
// Telemetry API is what the tests should be asserting against.
//
// Written in Go rather than as a shell script so it depends on nothing in the
// runtime image beyond the ability to execute a static binary.
//
// Stdlib only, so it builds without touching the extension's own module.
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

// otlpTraces is a single-line OTLP/JSON export request, as an SDK exporting to
// stdout would write it.
const otlpTraces = `{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"capture-func"}}]},"scopeSpans":[{"scope":{"name":"capture-instrumentation","version":"1.2.3"},"spans":[{"traceId":"5b8efff798038103d269b633813fc60c","spanId":"eee19b7ec3c1b174","name":"handler","kind":2,"startTimeUnixNano":"1753000000000000000","endTimeUnixNano":"1753000000123000000","attributes":[{"key":"http.status_code","value":{"intValue":"200"}}]}]}]}]}`

// otlpLogs is the log-signal equivalent.
const otlpLogs = `{"resourceLogs":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"capture-func"}}]},"scopeLogs":[{"logRecords":[{"timeUnixNano":"1753000000000000000","severityText":"ERROR","body":{"stringValue":"captured log record"}}]}]}]}`

// libhoneyEnvelope is what a beeline emits, and must keep working unchanged.
const libhoneyEnvelope = `{"time":"2025-07-20T08:26:40.000Z","samplerate":1,"data":{"name":"beeline-span","duration_ms":12.5}}`

func main() {
	runtimeAPI := os.Getenv("AWS_LAMBDA_RUNTIME_API")
	if runtimeAPI == "" {
		log.Fatal("AWS_LAMBDA_RUNTIME_API is not set")
	}

	// Only the first invocation emits payloads. Later invocations exist purely to
	// thaw the environment so the first one's telemetry gets delivered and
	// platform.report is emitted, and they must stay silent so the capture holds
	// exactly one of each shape however many times the script invokes.
	first := true
	for {
		requestID, err := nextInvocation(runtimeAPI)
		if err != nil {
			log.Fatalf("next invocation: %v", err)
		}

		if first {
			emitPayloads()
			first = false
		}

		if err := respond(runtimeAPI, requestID); err != nil {
			log.Fatalf("responding: %v", err)
		}
	}
}

// emitPayloads writes one line per shape under test. Each is prefixed on the
// line before with a marker, so a capture can be attributed even if Lambda
// reorders or rewraps the lines themselves.
func emitPayloads() {
	envelope, err := stdoutEnvelope()
	if err != nil {
		log.Printf("building envelope: %v", err)
	}

	payloads := []struct {
		name string
		line string
	}{
		{"otlp-traces", otlpTraces},
		{"otlp-logs", otlpLogs},
		{"otlp-stdout-envelope", envelope},
		{"libhoney-envelope", libhoneyEnvelope},
		{"plain-text", "an ordinary log line"},
	}

	for _, payload := range payloads {
		fmt.Printf("MARKER %s\n", payload.name)
		fmt.Println(payload.line)
	}
}

// stdoutEnvelope builds the line the otlp-stdout family of exporters emits:
// a compressed, base64-encoded export request wrapped in a descriptive envelope.
func stdoutEnvelope() (string, error) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write([]byte(otlpTraces)); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	line, err := json.Marshal(map[string]interface{}{
		"__otel_otlp_stdout": "otlp-stdout-span-exporter@0.15.0",
		"source":             "capture-func",
		"endpoint":           "http://localhost:4318/v1/traces",
		"method":             "POST",
		"content-type":       "application/json",
		"content-encoding":   "gzip",
		"payload":            base64.StdEncoding.EncodeToString(compressed.Bytes()),
		"base64":             true,
	})
	if err != nil {
		return "", err
	}
	return string(line), nil
}

func nextInvocation(runtimeAPI string) (string, error) {
	url := fmt.Sprintf("http://%s/2018-06-01/runtime/invocation/next", runtimeAPI)
	response, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if _, err := io.ReadAll(response.Body); err != nil {
		return "", err
	}
	return response.Header.Get("Lambda-Runtime-Aws-Request-Id"), nil
}

func respond(runtimeAPI, requestID string) error {
	url := fmt.Sprintf("http://%s/2018-06-01/runtime/invocation/%s/response", runtimeAPI, requestID)
	response, err := http.Post(url, "application/json", bytes.NewReader([]byte(`"captured"`)))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, err = io.ReadAll(response.Body)
	return err
}
