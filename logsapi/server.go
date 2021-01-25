package logsapi

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	libhoney "github.com/honeycombio/libhoney-go"
	log "github.com/sirupsen/logrus"
)

// LogMessage is a message sent from the Logs API
type LogMessage struct {
	Type   string      `json:"type"`
	Time   string      `json:"time"`
	Record interface{} `json:"record"`
}

// handler receives batches of log messages from the Lambda Logs API. Each
// LogMessage is sent to Honeycomb as a separate event.
func handler(libhoneyClient *libhoney.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Warn("Error", err)
			return
		}
		defer r.Body.Close()

		// The Logs API will send batches of events as an array of JSON objects.
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

		// Iterate through the batch of log messages received. In the case of function
		// log messages, the Record field of the struct will be a string. That string
		// may contain string-encoded JSON (e.g. "{\"trace.trace_id\": \"1234\", ...}")
		// in which case, we will try to parse the JSON into a map[string]interface{}
		// and then add it to the Honeycomb event. If, for some reason, parsing the JSON
		// is impossible, then we just add the string as a field "record" in Honeycomb.
		// This is what will happen if the function emits plain, non-structured strings.
		for _, msg := range logs {
			event := libhoneyClient.NewEvent()
			event.AddField("lambda_extension.type", msg.Type)

			switch v := msg.Record.(type) {
			case string:
				// attempt to parse as json
				var record map[string]interface{}
				err := json.Unmarshal([]byte(v), &record)
				if err != nil {
					event.Timestamp = parseMessageTimestamp(event, msg)
					event.AddField("record", msg.Record)
				} else {
					event.Timestamp = parseFunctionTimestamp(msg, record)
					event.Add(record)
				}
			default:
				// In the case of platform.start and platform.report messages, msg.Record
				// will be a map[string]interface{}.
				event.Timestamp = parseMessageTimestamp(event, msg)
				event.Add(msg.Record)
			}
			event.Send()
		}
	}
}

// parseMessageTimestamp is a helper function that tries to parse the timestamp from the
// log event payload. If it cannot parse the timestamp, it returns the current timestamp.
func parseMessageTimestamp(event *libhoney.Event, msg LogMessage) time.Time {
	ts, err := time.Parse(time.RFC3339, msg.Time)
	if err != nil {
		event.AddField("lambda_extension.time", msg.Time)
		return time.Now()
	}
	return ts
}

// parseFunctionTimestamp is a helper function that will return a timestamp for a function log message.
// There are some precedence rules:
//
// 1. Look for a "timestamp" field in the message body.
// 2. If not present, look for a "duration_ms" field and subtract it from the log event
//    timestamp.
// 3. If neither are present, just use the log timestamp.
func parseFunctionTimestamp(msg LogMessage, body map[string]interface{}) time.Time {

	// parse the logs API event time in case we need it. If it's invalid, just take the time now.
	messageTime, err := time.Parse(time.RFC3339, msg.Time)
	if err != nil {
		messageTime = time.Now()
	}

	ts, ok := body["timestamp"]
	if ok {
		strTs, okStr := ts.(string)
		if okStr {
			parsed, err := time.Parse(time.RFC3339, strTs)
			if err == nil {
				return parsed
			}
		}
	}

	dur, ok := body["duration_ms"]

	if ok {
		// duration_ms may be a float (e.g. 43.23), integer (e.g. 54) or a string (e.g. "43")
		switch duration := dur.(type) {
		case float64:
			if d, err := time.ParseDuration(fmt.Sprintf("%.4fms", duration)); err == nil {
				return messageTime.Add(-1 * d)
			}
		case int64:
			return messageTime.Add(-1 * (time.Duration(duration) * time.Millisecond))
		case string:
			if d, err := strconv.ParseFloat(duration, 64); err == nil {
				return messageTime.Add(-1 * (time.Duration(d) * time.Millisecond))
			}
		}
	}

	return messageTime
}

// StartHTTPServer starts a logs API server on the specified port. The server will receive
// log messages from the Lambda Logs API and send them to Honeycomb as events.
func StartHTTPServer(port int, client *libhoney.Client) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handler(client))
	server := &http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%d", port),
		Handler: mux,
	}
	log.Info("Logs server listening on port ", port)
	log.Fatal(server.ListenAndServe())
}
