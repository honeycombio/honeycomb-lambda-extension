// Package otlpjson recognizes OTLP/JSON export requests written to stdout by an
// OpenTelemetry SDK and translates them into Honeycomb events.
//
// Translation is delegated to husky, the same library Honeycomb's OTLP ingest
// uses, so field naming and dataset routing match what the OTLP endpoint would
// have produced for the same payload.
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
	SignalNone Signal = iota
	SignalTraces
	SignalLogs
)

// Both spellings of each key are accepted, because the OTLP JSON encoding
// permits either.
type envelope struct {
	ResourceSpans          json.RawMessage `json:"resourceSpans"`
	ResourceSpansSnakeCase json.RawMessage `json:"resource_spans"`
	ResourceLogs           json.RawMessage `json:"resourceLogs"`
	ResourceLogsSnakeCase  json.RawMessage `json:"resource_logs"`
}

// Detect reports which OTLP signal, if any, a stdout record carries, keying off
// a top-level resourceSpans or resourceLogs field.
//
// An export request holds one signal, so a record carrying both is malformed;
// traces win and the log records are ignored.
func Detect(record []byte) Signal {
	var env envelope
	if err := json.Unmarshal(record, &env); err != nil {
		return SignalNone
	}
	switch {
	case env.ResourceSpans != nil || env.ResourceSpansSnakeCase != nil:
		return SignalTraces
	case env.ResourceLogs != nil || env.ResourceLogsSnakeCase != nil:
		return SignalLogs
	default:
		return SignalNone
	}
}

// Translate converts an OTLP/JSON export request into Honeycomb events grouped
// by destination dataset, matching how the OTLP endpoint would route the same
// payload. Note that husky routes the two signals differently: spans use the
// dataset argument only for classic keys, while log records prefer their
// service.name for any key and fall back to the argument.
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
