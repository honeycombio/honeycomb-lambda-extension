package telemetryapi

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	libhoney "github.com/honeycombio/libhoney-go"
	logrus "github.com/sirupsen/logrus"
)

// LogMessage is an Event record sent from the Telemetry API
type LogMessage struct {
	Type   string      `json:"type"`
	Time   string      `json:"time"`
	Record interface{} `json:"record"`
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
// LogMessage is sent to Honeycomb as a separate event.
func handler(client eventCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Debug("handler - log batch received")
		body, err := ioutil.ReadAll(r.Body)
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

		// Iterate through the batch of log messages received. A function log
		// message's Record holds whatever the function wrote to stdout, in one
		// of two encodings. With plain-text log format (and all Logs API /
		// pre-2022-12-13 schema deliveries), Record is a string that may itself
		// contain JSON. With JSON log format, Lambda pre-parses the line:
		// a line that was already JSON arrives as that object verbatim, and a
		// non-JSON line arrives wrapped as {timestamp, level, message}.
		// Normalize all of these into the same structured handling so a span
		// emitted by libhoney/beeline parses identically regardless of the
		// function's logging config.
		for _, msg := range logs {
			event := client.NewEvent()
			event.AddField("lambda_extension.type", msg.Type)

			switch record := msg.Record.(type) {
			case string:
				addRecordString(event, msg, record)
			case map[string]interface{}:
				if inner, ok := record["message"].(string); ok && record["data"] == nil {
					// JSON-log-format wrapper around a non-JSON line; unwrap and
					// handle the original line as if it had arrived unwrapped.
					addRecordString(event, msg, inner)
				} else {
					addRecordJSON(event, msg, record)
				}
			default:
				event.Timestamp = parseMessageTimestamp(event, msg)
				event.Add(msg.Record)
			}
			event.Metadata, _ = event.Fields()["name"]
			event.SendPresampled()
			log.Debug("handler - event enqueued")
		}
	}
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
		event.Add(data)
	default:
		// data is not a map, so treat the record as flat JSON adding all keys as fields
		event.Add(jsonRecord)
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
func StartTelemetryReceiver(port int, client eventCreator) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handler(client))
	server := &http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%d", port),
		Handler: mux,
	}
	log.Info("Telemetry server listening on port ", port)
	log.Fatal(server.ListenAndServe())
}
