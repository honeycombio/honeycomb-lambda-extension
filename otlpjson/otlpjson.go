// Package otlpjson recognizes OTLP/JSON export requests written to stdout by an
// OpenTelemetry SDK and translates them into Honeycomb events.
//
// Translation is delegated to husky, the same library Honeycomb's OTLP ingest
// uses, so spans that arrive this way are indistinguishable from spans sent to
// the OTLP endpoint directly.
package otlpjson

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/honeycombio/husky/otlp"
)

// Signal identifies which OTLP export request a record holds, if any.
type Signal int

const (
	// SignalNone means the record is not an OTLP export request.
	SignalNone Signal = iota
	SignalTraces
	SignalLogs
)

// envelope holds only the top-level keys that identify an export request.
// Both the camelCase and snake_case spellings are accepted, because the OTLP
// JSON encoding permits either.
type envelope struct {
	ResourceSpans     json.RawMessage `json:"resourceSpans"`
	ResourceSpansSnek json.RawMessage `json:"resource_spans"`
	ResourceLogs      json.RawMessage `json:"resourceLogs"`
	ResourceLogsSnek  json.RawMessage `json:"resource_logs"`
}

// Detect reports which OTLP signal, if any, a stdout record carries. The
// presence of a resourceSpans or resourceLogs key is treated as definitive:
// no other producer of function stdout uses those names at the top level.
func Detect(record []byte) Signal {
	var env envelope
	if err := json.Unmarshal(record, &env); err != nil {
		return SignalNone
	}
	switch {
	case env.ResourceSpans != nil || env.ResourceSpansSnek != nil:
		return SignalTraces
	case env.ResourceLogs != nil || env.ResourceLogsSnek != nil:
		return SignalLogs
	default:
		return SignalNone
	}
}

// Translate converts an OTLP/JSON export request into Honeycomb events grouped
// by destination dataset. The dataset argument is only consulted for classic
// API keys; for Environments & Services keys husky derives the dataset from the
// telemetry itself, as it does for OTLP traffic arriving at the API.
func Translate(ctx context.Context, signal Signal, record []byte, apiKey, dataset string) ([]otlp.Batch, error) {
	ri := otlp.RequestInfo{
		ApiKey:      apiKey,
		Dataset:     dataset,
		ContentType: "application/json",
	}

	var (
		result *otlp.TranslateOTLPRequestResult
		err    error
	)
	body := io.NopCloser(bytes.NewReader(record))
	switch signal {
	case SignalTraces:
		result, err = otlp.TranslateTraceRequestFromReader(ctx, body, ri)
	case SignalLogs:
		result, err = otlp.TranslateLogsRequestFromReader(ctx, body, ri)
	default:
		return nil, fmt.Errorf("not an OTLP record")
	}
	if err != nil {
		return nil, err
	}
	return result.Batches, nil
}
