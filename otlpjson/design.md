# OTLP/JSON on stdout — design

## Goal

Let a function instrumented with plain OpenTelemetry emit OTLP/JSON to stdout and
have the extension deliver it to Honeycomb, without the app opening a socket to a
collector sidecar. The extension keeps its current shape: it reads what the
function already writes to stdout, via the Telemetry API. No new listeners, no
collector.

## Wire format accepted

A single-line JSON object whose top level is an OTLP export request:

- traces: `{"resourceSpans": [...]}` (or `resource_spans`)
- logs:   `{"resourceLogs": [...]}` (or `resource_logs`)

This is the OTLP/JSON (protojson) encoding — what Java's
`OtlpJsonLoggingSpanExporter` and the OTLP File exporter emit. Both camelCase and
snake_case key styles are accepted because protojson accepts both.

Out of scope for this change: SDK-specific console exporters (Go `stdouttrace`,
Python/JS `ConsoleSpanExporter`) emit bespoke shapes, not OTLP; gzip+base64
framing; metrics.

## Detection

A function log record is treated as OTLP if, after the existing normalization
into a `map[string]interface{}`, it has a `resourceSpans`/`resource_spans` or
`resourceLogs`/`resource_logs` key. That discriminator is unambiguous against
libhoney/beeline envelopes and against arbitrary user JSON, so there is no
configuration flag and no behavior change for existing users.

If a record looks like OTLP but fails to translate, it falls through to the
existing handling rather than being dropped — a malformed span still reaches
Honeycomb as a log line, which is what a user debugging this needs to see.

## Translation

`husky/otlp` is the same library Honeycomb's OTLP ingest uses.
`TranslateTraceRequestFromReader` / `TranslateLogsRequestFromReader` take the raw
JSON bytes plus a `RequestInfo{ContentType: "application/json", ApiKey, Dataset}`
and return batches of Honeycomb events. Reusing it means field naming, resource
attribute flattening, sample rate, and timestamps are identical to what the same
spans would produce through the OTLP endpoint — there is no second mapping to
keep in sync.

## Dataset routing

Events are sent with husky's per-batch `Dataset`, overriding the client default.
For Environments & Services keys that is the span's `service.name`, matching
direct OTLP ingest. For classic keys husky falls back to the configured
`LIBHONEY_DATASET`, which is also the classic-key requirement. Non-OTLP records
are unaffected and continue to go to `LIBHONEY_DATASET`.

## Fan-out

The Telemetry API handler is currently one log message to one event. An OTLP
record carries many spans, so the per-record path returns a slice of events.

## Operational constraints to document

- The payload must be one line. Pretty-printed JSON arrives as several unrelated
  log records and cannot be reassembled.
- Lambda truncates long log lines, so a large batch is lost rather than split.
  Guidance is to keep batches small.
