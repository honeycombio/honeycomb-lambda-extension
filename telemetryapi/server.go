package telemetryapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/honeycombio/honeycomb-lambda-extension/extension"
	"github.com/honeycombio/honeycomb-lambda-extension/otlpjson"
	libhoney "github.com/honeycombio/libhoney-go"
	logrus "github.com/sirupsen/logrus"
)

// LogMessage is an Event record sent from the Telemetry API.
//
// Record is held as raw bytes rather than a decoded value so that a record
// carrying OTLP/JSON can be handed to the translator exactly as the function
// wrote it. Decoding to interface{} first would round-trip nanosecond
// timestamps through float64 and lose precision.
type LogMessage struct {
	Type   string          `json:"type"`
	Time   string          `json:"time"`
	Record json.RawMessage `json:"record"`
}

type eventCreator interface {
	NewEvent() *libhoney.Event
}

var (
	// set up logging defaults for our own logging output
	log = logrus.WithFields(logrus.Fields{
		"source": "hny-lambda-ext-telemetryapi",
	})
)

// handler receives batches of log messages from the Lambda Telemetry API. Each
// LogMessage is sent to Honeycomb as a separate event, except for records
// holding OTLP/JSON, which expand into one event per span or log record.
func handler(client eventCreator, config extension.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Debug("handler - log batch received")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Warn("Error", err)
			return
		}
		defer r.Body.Close()

		// The Telemetry API will send batches of events as an array of JSON objects.
		// Each object will have time, type and record as the top-level keys. If
		// the log message is a function message, the record element will contain
		// whatever was emitted by the function to stdout. This could be a structured
		// log message (JSON) or a plain string.
		var logs []LogMessage
		err = json.Unmarshal(body, &logs)
		if err != nil {
			log.Warn("Could not unmarshal payload", err)
			return
		}

		for _, msg := range logs {
			handleMessage(client, config, msg)
		}
	}
}

// handleMessage turns one message from the batch into events. A record holding
// an OTLP export request expands into one event per span or log record; every
// other record becomes a single event.
//
// A function log message's Record holds whatever the function wrote to stdout,
// in one of two encodings. With plain-text log format (and all Logs API /
// pre-2022-12-13 schema deliveries), Record is a string that may itself
// contain JSON. With JSON log format, Lambda pre-parses the line:
// a line that was already JSON arrives as that object verbatim, and a
// non-JSON line arrives wrapped as {timestamp, level, message}.
// Normalize all of these into the same structured handling so a span
// emitted by libhoney/beeline parses identically regardless of the
// function's logging config.
func handleMessage(client eventCreator, config extension.Config, msg LogMessage) {
	line, isLine := stdoutLine(msg.Record)
	stdout := msg.Record
	if isLine {
		stdout = json.RawMessage(line)
	}

	if sendOTLP(client, config, msg, stdout) {
		return
	}

	event := newEvent(client, msg)
	if isLine {
		addRecordString(event, msg, line)
		sendEvent(event)
		return
	}

	// A record that is absent or null leaves record nil, and the message still
	// reaches Honeycomb carrying its type and timestamp.
	var record interface{}
	if len(msg.Record) > 0 {
		if err := json.Unmarshal(msg.Record, &record); err != nil {
			// Syntactically valid JSON can still fail to decode: a number
			// beyond float64's range, for one. Every message the platform
			// delivers should reach Honeycomb in some form, so report the
			// record as the raw line it was rather than dropping it.
			log.Warn("Could not unmarshal record", err)
			event.Timestamp = parseMessageTimestamp(event, msg)
			event.AddField("record", string(msg.Record))
			sendEvent(event)
			return
		}
	}

	if fields, ok := record.(map[string]interface{}); ok {
		addRecordJSON(event, msg, fields)
	} else {
		event.Timestamp = parseMessageTimestamp(event, msg)
		if record != nil {
			event.Add(record)
		}
	}
	sendEvent(event)
}

var messageKey = []byte(`"message"`)

// stdoutLine returns the stdout line a record holds, if it holds a line rather
// than structured data: the record itself in plain-text log format, or the
// message field of the {timestamp, level, message} wrapper that JSON log format
// puts around a non-JSON line.
//
// Decided from the raw bytes rather than a decoded record, so that an export
// request reaches the translator without first being built into an object graph
// that is then discarded.
func stdoutLine(record json.RawMessage) (string, bool) {
	switch firstToken(record) {
	case '"':
		var line string
		if err := json.Unmarshal(record, &line); err != nil {
			return "", false
		}
		return line, true
	case '{':
		// A byte scan, so it errs in both directions and neither is a
		// correctness problem. A payload merely containing the key -- an OTLP
		// log record with a message attribute, say -- still takes the decode
		// below and is rejected by the shape check, so the saving is real only
		// for records that never mention it. A wrapper that spelled the key
		// with an escape would be missed here, which the platform that
		// generates the wrapper does not do.
		if !bytes.Contains(record, messageKey) {
			return "", false
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(record, &fields); err != nil {
			return "", false
		}
		return wrappedLine(fields)
	}
	return "", false
}

// wrappedLine reports the line a JSON-log-format wrapper carries. The wrapper is
// exactly {timestamp, level, message}, plus a requestId from the managed
// runtimes. A function's own structured log can carry a message field too, and
// unwrapping that would keep the message and silently discard every field
// beside it, so only the full wrapper shape unwraps.
func wrappedLine(fields map[string]json.RawMessage) (string, bool) {
	for key := range fields {
		switch key {
		case "message", "timestamp", "level", "requestId":
		default:
			return "", false
		}
	}
	// A null message is rejected explicitly, because unmarshalling a JSON null
	// into a string is a no-op that reports no error: without this it would
	// unwrap to an empty line and discard the record.
	if isNull(fields["message"]) {
		return "", false
	}
	var message string
	if err := json.Unmarshal(fields["message"], &message); err != nil {
		return "", false
	}
	return message, true
}

// isNull reports whether a field is absent or explicitly null, which the wrapper
// check treats alike.
func isNull(value json.RawMessage) bool {
	return len(value) == 0 || bytes.Equal(value, []byte("null"))
}

// firstToken returns the first meaningful byte of a JSON value, which is enough
// to tell a string from an object without decoding either.
func firstToken(value json.RawMessage) byte {
	for _, b := range value {
		switch b {
		case ' ', '\t', '\r', '\n':
		default:
			return b
		}
	}
	return 0
}

// newEvent starts an event, marking which kind of telemetry it came from.
//
// Translated OTLP is marked too, even though the OTLP endpoint would not add
// this field. Annotating telemetry with the component that processed it is the
// same thing Refinery does on the way through, and it is how a query can tell a
// span that arrived via this extension from one sent directly.
func newEvent(client eventCreator, msg LogMessage) *libhoney.Event {
	event := client.NewEvent()
	event.AddField("lambda_extension.type", msg.Type)
	return event
}

func sendEvent(event *libhoney.Event) {
	event.Metadata, _ = event.Fields()["name"]
	event.SendPresampled()
	log.Debug("handler - event enqueued")
}

// sendOTLP checks whether a function's stdout line is an OTLP/JSON export
// request and, if so, sends every span or log record it contains. It reports
// whether the line was handled.
//
// A line that looks like OTLP but fails to translate is deliberately not
// handled here: falling through to the ordinary log handling puts the offending
// payload in front of the user, which is what someone debugging their exporter
// configuration needs to see.
func sendOTLP(client eventCreator, config extension.Config, msg LogMessage, record []byte) bool {
	if msg.Type != string(FunctionLog) {
		return false
	}
	payload, err := otlpjson.Parse(record)
	if err != nil {
		log.WithError(err).Warn("Could not read OTLP record from function stdout")
		return false
	}
	if payload == nil {
		return false
	}

	batches, err := otlpjson.Translate(context.Background(), payload, config.APIKey, config.Dataset)
	if err != nil {
		log.WithError(err).Warn("Could not translate OTLP record from function stdout")
		return false
	}

	sent := 0
	for _, batch := range batches {
		for _, translated := range batch.Events {
			event := newEvent(client, msg)
			event.Dataset = batch.Dataset
			event.Timestamp = translated.Timestamp
			event.SampleRate = sampleRate(translated.SampleRate)
			event.Add(translated.Attributes)
			sendEvent(event)
			sent++
		}
	}

	// An export request holding no spans or log records would otherwise vanish
	// without a trace, being neither translated into anything nor logged.
	if sent == 0 {
		log.Warn("OTLP record from function stdout contained no spans or log records")
		return false
	}
	return true
}

// sampleRate converts the rate husky derived into the one libhoney takes,
// enforcing a floor that husky does not. A sample rate is meaningless below 1,
// and a non-positive one converted to an unsigned type becomes an
// astronomically large weight rather than a small one, so every event in the
// batch would be counted as millions.
func sampleRate(derived int32) uint {
	if derived < 1 {
		return 1
	}
	return uint(derived)
}

// addRecordString populates event from a raw log line, parsing it as JSON when
// possible and falling back to a timestamped "record" string field.
func addRecordString(event *libhoney.Event, msg LogMessage, record string) {
	var jsonRecord map[string]interface{}
	if err := json.Unmarshal([]byte(record), &jsonRecord); err != nil {
		event.Timestamp = parseMessageTimestamp(event, msg)
		event.AddField("record", record)
		return
	}
	addRecordJSON(event, msg, jsonRecord)
}

// addRecordJSON populates event from a structured record: fields come from the
// libhoney envelope's data map when present, otherwise from the record itself.
func addRecordJSON(event *libhoney.Event, msg LogMessage, jsonRecord map[string]interface{}) {
	event.Timestamp = parseFunctionTimestamp(msg, jsonRecord)
	switch data := jsonRecord["data"].(type) {
	case map[string]interface{}:
		// data key contains a map, likely emitted by a Beeline's libhoney, so add the fields from it
		event.AddFields(data)
	default:
		// data is not a map, so treat the record as flat JSON adding all keys as fields
		event.AddFields(jsonRecord)
	}
	event.SampleRate = parseSampleRate(jsonRecord)
}

// parseMessageTimestamp is a helper function that tries to parse the timestamp from the
// log event payload. If it cannot parse the timestamp, it returns the current timestamp.
func parseMessageTimestamp(event *libhoney.Event, msg LogMessage) time.Time {
	log.Debug("parseMessageTimestamp")
	ts, err := time.Parse(time.RFC3339, msg.Time)
	if err != nil {
		event.AddField("lambda_extension.time", msg.Time)
		return time.Now()
	}
	return ts
}

func parseSampleRate(body map[string]interface{}) uint {
	rate, ok := body["samplerate"]
	var foundRate int

	if ok {
		// samplerate may be a float (e.g. 43.23), integer (e.g. 54) or a string (e.g. "43")
		switch sampleRate := rate.(type) {
		case float64:
			foundRate = int(sampleRate)
		case int64:
			foundRate = int(sampleRate)
		case string:
			if d, err := strconv.Atoi(sampleRate); err == nil {
				foundRate = d
			}
		}
	}
	if foundRate < 1 {
		return 1
	}
	return uint(foundRate)
}

// parseFunctionTimestamp is a helper function that will return a timestamp for a function log message.
// There are some precedence rules:
//
//  1. Look for a "time" field from a libhoney transmission in the message body.
//  2. Look for a "timestamp" field in the message body.
//  3. If not present, look for a "duration_ms" field and subtract it from the log event
//     timestamp.
//  4. If neither are present, just use the log timestamp.
func parseFunctionTimestamp(msg LogMessage, body map[string]interface{}) time.Time {
	log.Debug("parseFunctionTimestamp")

	libhoneyTs, ok := body["time"]
	if ok {
		strLibhoneyTs, okStr := libhoneyTs.(string)
		if okStr {
			parsed, err := time.Parse(time.RFC3339, strLibhoneyTs)
			if err == nil {
				log.Debug("Timestamp from 'time'")
				return parsed
			}
		}
	}

	ts, ok := body["timestamp"]
	if ok {
		strTs, okStr := ts.(string)
		if okStr {
			parsed, err := time.Parse(time.RFC3339, strTs)
			if err == nil {
				log.Debug("Timestamp from 'timestamp'")
				return parsed
			}
		}
	}

	// parse the telemetry event time in case we need it. If it's invalid, just take the time now.
	messageTime, err := time.Parse(time.RFC3339, msg.Time)
	if err != nil {
		log.Debug("Unable to parse message's Time, defaulting to Now()")
		messageTime = time.Now()
	} else {
		log.Debug("Using message's Time field.")
	}

	dur, ok := body["duration_ms"]
	if ok {
		// duration_ms may be a float (e.g. 43.23), integer (e.g. 54) or a string (e.g. "43")
		switch duration := dur.(type) {
		case float64:
			if d, err := time.ParseDuration(fmt.Sprintf("%.4fms", duration)); err == nil {
				log.Debug("Timestamp computed from a float64 'duration_ms'")
				return messageTime.Add(-1 * d)
			}
		case int64:
			log.Debug("Timestamp computed from an int64 'duration_ms'")
			return messageTime.Add(-1 * (time.Duration(duration) * time.Millisecond))
		case string:
			if d, err := strconv.ParseFloat(duration, 64); err == nil {
				log.Debug("Timestamp computed from a string 'duration_ms'")
				return messageTime.Add(-1 * (time.Duration(d) * time.Millisecond))
			}
		}
	}

	return messageTime
}

// StartTelemetryReceiver starts a small HTTP server on the specified port.
// The server receives log messages in AWS Lambda's [Telemetry API message format]
// (JSON array of messages) and the handler will send them to Honeycomb
// as events with the eventCreator provided as client.
//
// When running in Lambda, the extension's subscription to telemetry types will
// result in the Lambda Telemetry API publishing log messages to this receiver.
//
// When running in localMode, the server will be started for manual posting of
// log messages to the specified port for testing.
//
// [Telemetry API message format]: https://docs.aws.amazon.com/lambda/latest/dg/telemetry-api.html#telemetry-api-messages
func StartTelemetryReceiver(config extension.Config, client eventCreator) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handler(client, config))
	server := &http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%d", config.LogsReceiverPort),
		Handler: mux,
	}
	log.Info("Telemetry server listening on port ", config.LogsReceiverPort)
	log.Fatal(server.ListenAndServe())
}
