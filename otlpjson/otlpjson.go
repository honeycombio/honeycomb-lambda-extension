// Package otlpjson recognizes OpenTelemetry export requests written to stdout
// and translates them into Honeycomb events.
//
// Translation is delegated to husky, the same library Honeycomb's OTLP ingest
// uses, so field naming and dataset routing match what the OTLP endpoint would
// have produced for the same payload.
package otlpjson

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/honeycombio/husky/otlp"
)

// Signal identifies which OTLP export request a record holds.
type Signal int

const (
	SignalNone Signal = iota
	SignalTraces
	SignalLogs
)

// Payload is an OTLP export request recovered from a line of function stdout,
// described well enough for husky to decode it.
type Payload struct {
	Signal          Signal
	Body            []byte
	ContentType     string
	ContentEncoding string
}

const jsonContentType = "application/json"

// presence records that a key was there without keeping its value, so
// recognizing a signal costs a scan rather than a copy of the whole export
// request. A key present but null still counts as present, matching how the
// signal keys are used: their presence names the signal, their contents are the
// translator's business.
//
// It only ever latches true, so an exportRequest must be decoded once and
// discarded rather than reused across records.
type presence bool

func (p *presence) UnmarshalJSON([]byte) error {
	*p = true
	return nil
}

// Both spellings of each key are accepted, because the OTLP JSON encoding
// permits either.
type exportRequest struct {
	ResourceSpans          presence `json:"resourceSpans"`
	ResourceSpansSnakeCase presence `json:"resource_spans"`
	ResourceLogs           presence `json:"resourceLogs"`
	ResourceLogsSnakeCase  presence `json:"resource_logs"`

	// Set only by the otlp-stdout family of exporters, which wrap a compressed
	// export request rather than writing OTLP/JSON directly.
	Marker          string `json:"__otel_otlp_stdout"`
	Endpoint        string `json:"endpoint"`
	ContentType     string `json:"content-type"`
	ContentEncoding string `json:"content-encoding"`
	EnvelopePayload string `json:"payload"`
	IsBase64        bool   `json:"base64"`
}

// Parse recovers the OTLP export request a stdout line carries, in either of the
// two forms an SDK writes: OTLP/JSON directly, or the otlp-stdout exporters'
// envelope wrapping a compressed, usually base64-encoded payload.
//
// A nil Payload with a nil error means the line is not an export request at all,
// which is the common case for ordinary log output. An error means the line
// announced itself as one but could not be read.
func Parse(record []byte) (*Payload, error) {
	var request exportRequest
	if err := json.Unmarshal(record, &request); err != nil {
		return nil, nil
	}

	if request.Marker != "" {
		return parseEnvelope(request)
	}

	if request.ResourceSpans || request.ResourceSpansSnakeCase {
		return &Payload{Signal: SignalTraces, Body: record, ContentType: jsonContentType}, nil
	}
	if request.ResourceLogs || request.ResourceLogsSnakeCase {
		return &Payload{Signal: SignalLogs, Body: record, ContentType: jsonContentType}, nil
	}
	return nil, nil
}

// parseEnvelope unwraps an otlp-stdout envelope. The content type and encoding
// it declares are passed through for husky to validate, so that a payload
// written as protobuf, JSON, gzip, zstd or uncompressed all work without this
// package tracking the exporter's defaults.
func parseEnvelope(request exportRequest) (*Payload, error) {
	signal, err := signalFromEndpoint(request.Endpoint)
	if err != nil {
		return nil, err
	}

	body := []byte(request.EnvelopePayload)
	if request.IsBase64 {
		body, err = base64.StdEncoding.DecodeString(request.EnvelopePayload)
		if err != nil {
			return nil, fmt.Errorf("decoding base64 payload: %w", err)
		}
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("envelope carries an empty payload")
	}

	return &Payload{
		Signal:          signal,
		Body:            body,
		ContentType:     request.ContentType,
		ContentEncoding: request.ContentEncoding,
	}, nil
}

// signalFromEndpoint reads the signal from the OTLP endpoint the envelope was
// addressed to, since a compressed payload can't be inspected for it. These
// exporters default to the traces endpoint and may omit the field.
func signalFromEndpoint(endpoint string) (Signal, error) {
	path := strings.TrimRight(endpoint, "/")
	switch {
	case path == "" || strings.HasSuffix(path, "/v1/traces"):
		return SignalTraces, nil
	case strings.HasSuffix(path, "/v1/logs"):
		return SignalLogs, nil
	default:
		return SignalNone, fmt.Errorf("unsupported OTLP endpoint %q", endpoint)
	}
}

// Translate converts an export request into Honeycomb events grouped by
// destination dataset, matching how the OTLP endpoint would route the same
// payload. Note that husky routes the two signals differently: spans use the
// dataset argument only for classic keys, while log records prefer their
// service.name for any key and fall back to the argument.
func Translate(ctx context.Context, payload *Payload, apiKey, dataset string) ([]otlp.Batch, error) {
	ri := otlp.RequestInfo{
		ApiKey:          apiKey,
		Dataset:         dataset,
		ContentType:     payload.ContentType,
		ContentEncoding: payload.ContentEncoding,
	}

	var (
		result *otlp.TranslateOTLPRequestResult
		err    error
	)
	body := io.NopCloser(bytes.NewReader(payload.Body))
	switch payload.Signal {
	case SignalTraces:
		result, err = otlp.TranslateTraceRequestFromReader(ctx, body, ri)
	case SignalLogs:
		result, err = otlp.TranslateLogsRequestFromReader(ctx, body, ri)
	default:
		return nil, fmt.Errorf("not an OTLP payload")
	}
	if err != nil {
		return nil, err
	}
	return result.Batches, nil
}
