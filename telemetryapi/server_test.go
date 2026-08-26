package telemetryapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/honeycombio/honeycomb-lambda-extension/extension"
	libhoney "github.com/honeycombio/libhoney-go"
	"github.com/honeycombio/libhoney-go/transmission"
	"github.com/stretchr/testify/assert"
)

var (
	epochTimestamp     = "1970-01-01T01:01:01.010Z"
	christmasTimestamp = "2020-12-25T12:34:56.789Z"

	platformStartMessage = LogMessage{
		Time:   "2020-11-03T21:10:25.133Z",
		Type:   "platform.start",
		Record: rec(`{"requestId": "6d67e385-053d-4622-a56f-b25bcef23083", "version": "$LATEST"}`),
	}

	nonJsonFunctionMessage = LogMessage{
		Time:   "2020-11-03T21:10:25.150Z",
		Type:   "function",
		Record: recString("A basic message to STDOUT"),
	}

	functionMessageWithStringDurationNoTimestamp = LogMessage{
		Time:   "2020-11-03T21:10:25.150Z",
		Type:   "function",
		Record: recString("{\"foo\": \"bar\", \"duration_ms\": \"54\"}"),
	}

	functionMessageWithIntDurationNoTimestamp = LogMessage{
		Time:   "2020-11-03T21:10:25.150Z",
		Type:   "function",
		Record: recString("{\"foo\": \"bar\", \"duration_ms\": 54}"),
	}

	functionMessageWithFloatDurationNoTimestamp = LogMessage{
		Time:   "2020-11-03T21:10:25.150Z",
		Type:   "function",
		Record: recString("{\"foo\": \"bar\", \"duration_ms\": 54.43}"),
	}

	functionMessageWithTimestamp = LogMessage{
		Time:   "2020-11-03T21:10:25.150Z",
		Type:   "function",
		Record: recString("{\"foo\": \"bar\", \"duration_ms\": 54, \"timestamp\": \"2020-11-03T21:10:25.090Z\"}"),
	}

	functionMessageFromLibhoneyTransmission = LogMessage{
		Time: epochTimestamp,
		Type: "function",
		// 🎄
		Record: recString(`{"time": "2020-12-25T12:34:56.789Z", "samplerate": 1, "data": {"foo": "bar", "duration_ms": 54} }`),
	}

	functionMessageJsonAndDataIsNotMappable = LogMessage{
		Time:   epochTimestamp,
		Type:   "function",
		Record: recString(`{"timestamp": "2020-12-25T12:34:56.789Z", "data": "an android" }`),
	}

	logMessages = []LogMessage{
		platformStartMessage,
		nonJsonFunctionMessage,
		functionMessageWithStringDurationNoTimestamp,
		functionMessageWithIntDurationNoTimestamp,
		functionMessageWithTimestamp,
		functionMessageFromLibhoneyTransmission,
		functionMessageJsonAndDataIsNotMappable,
	}
)

// rec builds a Record from the JSON the Telemetry API would have delivered.
func rec(jsonText string) json.RawMessage {
	return json.RawMessage(jsonText)
}

// recString builds a Record for a stdout line delivered in plain-text log
// format, where the line arrives as a JSON string.
func recString(line string) json.RawMessage {
	encoded, err := json.Marshal(line)
	if err != nil {
		panic(err)
	}
	return encoded
}

func postMessages(t *testing.T, messages []LogMessage) []*transmission.Event {
	return postMessagesWithConfig(t, messages, extension.Config{})
}

func postMessagesWithConfig(t *testing.T, messages []LogMessage, config extension.Config) []*transmission.Event {
	b, err := json.Marshal(messages)
	if err != nil {
		t.Error(err)
	}
	return postBody(t, string(b), config)
}

// postBody posts a raw Telemetry API payload, for shapes that a []LogMessage
// can't express.
func postBody(t *testing.T, body string, config extension.Config) []*transmission.Event {
	rr := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "/", bytes.NewBufferString(body))
	if err != nil {
		t.Error(err)
	}
	testTx := &transmission.MockSender{}
	// Mirror the production publisher, which configures the client with the
	// dataset from the environment. Events only carry a dataset of their own
	// when something has deliberately overridden it.
	client, _ := libhoney.NewClient(libhoney.ClientConfig{
		Transmission: testTx,
		APIKey:       "blah",
		Dataset:      config.Dataset,
	})
	handler(client, config).ServeHTTP(rr, req)
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}
	return testTx.Events()
}

func TestLogMessage(t *testing.T) {
	events := postMessages(t, logMessages)

	assert.Equal(t, 7, len(events))

	assert.Equal(t, "platform.start", events[0].Data["lambda_extension.type"])
	assert.Equal(t, "function", events[1].Data["lambda_extension.type"])
	assert.Equal(t, "function", events[2].Data["lambda_extension.type"])
	assert.Equal(t, "function", events[3].Data["lambda_extension.type"])

	assert.Equal(t, "$LATEST", events[0].Data["version"])
	assert.Equal(t, "A basic message to STDOUT", events[1].Data["record"])
	assert.Equal(t, "bar", events[2].Data["foo"])
	assert.Equal(t, "bar", events[5].Data["foo"])
	assert.Equal(t, "an android", events[6].Data["data"])
}

func TestLogMessageFromLibhoneyTransmission(t *testing.T) {
	events := postMessages(t, []LogMessage{
		{
			Time: epochTimestamp,
			Type: "function",
			// 🎄
			Record: recString(`{"time": "2020-12-25T12:34:56.789Z", "samplerate": 1, "data": {"foo": "bar", "duration_ms": 54}, "foo": "BOGUS", "duration_ms": "ALSO BOGUS" }`),
		},
	})

	parsedEvent := events[0]

	ts, _ := time.Parse(time.RFC3339, christmasTimestamp)
	assert.Equal(t,
		ts.String(),
		parsedEvent.Timestamp.String(),
		"Want: 🎄! Do not want: epoch. The event's time should be from the time key within the Transmission JSON, not the Lambda Function's log timestamp.",
	)
	assert.Equal(t, "bar", parsedEvent.Data["foo"], "The foo and its value should have been found under the data key within the Transmission JSON.")
	assert.Equal(t, float64(54), parsedEvent.Data["duration_ms"], "The duration should have been found under the data key within the Transmission JSON.")
}

func TestLogMessageJsonWithUnmappableData(t *testing.T) {
	events := postMessages(t, []LogMessage{functionMessageJsonAndDataIsNotMappable})

	parsedEvent := events[0]

	assert.Equal(t, "an android", parsedEvent.Data["data"], "The Data map on the Event should contain a field named 'data' with a single value.")
}

func TestTimestampsFunctionMessageNoJson(t *testing.T) {
	events := postMessages(t, []LogMessage{nonJsonFunctionMessage})
	event := events[0]

	ts, _ := time.Parse(time.RFC3339, "2020-11-03T21:10:25.150Z")
	assert.Equal(t, ts.String(), event.Timestamp.String())
}

func TestTimestampsPlatformMessage(t *testing.T) {
	events := postMessages(t, []LogMessage{platformStartMessage})
	event := events[0]

	// try to parse the timestamp from the Time field of a platform message
	ts, err := time.Parse(time.RFC3339, "2020-11-03T21:10:25.133Z")
	if err != nil {
		assert.Fail(t, "Could not parse timestamp")
	}
	assert.Equal(t, ts.String(), event.Timestamp.String())
}

func TestTimestampsFunctionMessageWithTimestamp(t *testing.T) {
	events := postMessages(t, []LogMessage{functionMessageWithTimestamp})
	event := events[0]

	// try to parse the timestamp from the event body of a function message
	ts, err := time.Parse(time.RFC3339, "2020-11-03T21:10:25.090Z")
	if err != nil {
		assert.Fail(t, "Could not parse timestamp")
	}
	assert.Equal(t, ts.String(), event.Timestamp.String())
}

func TestTimestampsFunctionMessageWithDuration(t *testing.T) {
	events := postMessages(t, []LogMessage{
		functionMessageWithStringDurationNoTimestamp,
		functionMessageWithIntDurationNoTimestamp,
	})

	// when no timestamp is present in the body, take the event timestamp and subtract duration
	for _, event := range events {
		ts, err := time.Parse(time.RFC3339, "2020-11-03T21:10:25.150Z")
		if err != nil {
			assert.Fail(t, "Could not parse timestamp")
		}
		d := 54 * time.Millisecond
		ts = ts.Add(-1 * d)
		assert.Equal(t, ts.String(), event.Timestamp.String())
	}

	events = postMessages(t, []LogMessage{functionMessageWithFloatDurationNoTimestamp})
	event := events[0]

	ts, err := time.Parse(time.RFC3339, "2020-11-03T21:10:25.150Z")
	if err != nil {
		assert.Fail(t, "Could not parse timestamp")
	}
	d, _ := time.ParseDuration(fmt.Sprintf("%.4fms", 54.43))
	ts = ts.Add(-1 * d)
	assert.Equal(t, ts.String(), event.Timestamp.String())

}

func TestLibhoneyEventWithSampleRate(t *testing.T) {
	t.Run("integer", func(t *testing.T) {
		events := postMessages(t, []LogMessage{{
			Time:   epochTimestamp,
			Type:   "function",
			Record: recString(`{"time": "2020-12-25T12:34:56.789Z", "samplerate": 5, "data": {"foo": "bar", "duration_ms": 54} }`),
		}})
		event := events[0]
		assert.EqualValues(t, 5, event.SampleRate)
	})
	t.Run("float", func(t *testing.T) {
		events := postMessages(t, []LogMessage{{
			Time:   epochTimestamp,
			Type:   "function",
			Record: recString(`{"time": "2020-12-25T12:34:56.789Z", "samplerate": 10.1, "data": {"foo": "bar", "duration_ms": 54} }`),
		}})
		event := events[0]
		// we round downwards
		assert.EqualValues(t, 10, event.SampleRate)
	})
	t.Run("string", func(t *testing.T) {
		events := postMessages(t, []LogMessage{{
			Time:   epochTimestamp,
			Type:   "function",
			Record: recString(`{"time": "2020-12-25T12:34:56.789Z", "samplerate": "11", "data": {"foo": "bar", "duration_ms": 54} }`),
		}})
		event := events[0]
		assert.EqualValues(t, 11, event.SampleRate)
	})
	t.Run("string without a number", func(t *testing.T) {
		events := postMessages(t, []LogMessage{{
			Time:   epochTimestamp,
			Type:   "function",
			Record: recString(`{"time": "2020-12-25T12:34:56.789Z", "samplerate": "hello", "data": {"foo": "bar", "duration_ms": 54} }`),
		}})
		event := events[0]
		assert.EqualValues(t, 1, event.SampleRate)
	})
	t.Run("bool", func(t *testing.T) {
		events := postMessages(t, []LogMessage{{
			Time:   epochTimestamp,
			Type:   "function",
			Record: recString(`{"time": "2020-12-25T12:34:56.789Z", "samplerate": true, "data": {"foo": "bar", "duration_ms": 54} }`),
		}})
		event := events[0]
		assert.EqualValues(t, 1, event.SampleRate)
	})
	t.Run("negative number", func(t *testing.T) {
		events := postMessages(t, []LogMessage{{
			Time:   epochTimestamp,
			Type:   "function",
			Record: recString(`{"time": "2020-12-25T12:34:56.789Z", "samplerate": -12, "data": {"foo": "bar", "duration_ms": 54} }`),
		}})
		event := events[0]
		assert.EqualValues(t, 1, event.SampleRate)
	})
}

// Shapes delivered by the Telemetry API when the function's log format is
// JSON (schema 2022-12-13+, always the case on Lambda Managed Instances):
// a stdout line that was already JSON arrives pre-parsed as an object.
func TestJSONFormatRecordObjects(t *testing.T) {
	t.Run("beeline envelope arrives as pre-parsed object", func(t *testing.T) {
		events := postMessages(t, []LogMessage{{
			Time: epochTimestamp,
			Type: "function",
			Record: rec(`{
				"time": "2020-12-25T12:34:56.789Z",
				"dataset": "retriever-traces",
				"samplerate": 5,
				"data": {
					"name": "QueryKiller.Tick",
					"trace.trace_id": "97cac7afa949e6e0ccf399e11509c275",
					"duration_ms": 0.019234
				}
			}`),
		}})
		event := events[0]
		assert.Equal(t, "QueryKiller.Tick", event.Data["name"])
		assert.Equal(t, "97cac7afa949e6e0ccf399e11509c275", event.Data["trace.trace_id"])
		assert.NotContains(t, event.Data, "data", "envelope must be unwrapped, not double-encoded")
		ts, _ := time.Parse(time.RFC3339, christmasTimestamp)
		assert.Equal(t, ts.String(), event.Timestamp.String())
		assert.EqualValues(t, 5, event.SampleRate)
	})

	// The wrapper is the platform's, and a function's own log can look like it.
	// Unwrapping one of those keeps the message and loses everything else, and
	// only under JSON log format, so the same line would reach Honeycomb as two
	// different events depending on a setting the function did not choose.
	t.Run("a function's own structured log keeps all its fields", func(t *testing.T) {
		const line = `{"message": "user logged in", "user_id": 42}`

		asObject := postMessages(t, []LogMessage{{
			Time: epochTimestamp, Type: "function", Record: rec(line),
		}})[0]
		assert.Equal(t, "user logged in", asObject.Data["message"])
		assert.EqualValues(t, 42, asObject.Data["user_id"],
			"a message field alone must not be mistaken for the platform's wrapper")

		asLine := postMessages(t, []LogMessage{{
			Time: epochTimestamp, Type: "function", Record: recString(line),
		}})[0]
		assert.Equal(t, asObject.Data, asLine.Data,
			"the log format the function is configured with must not change the event")
	})

	t.Run("non-JSON line arrives wrapped in timestamp/level/message", func(t *testing.T) {
		events := postMessages(t, []LogMessage{{
			Time: "2020-11-03T21:10:25.150Z",
			Type: "function",
			Record: rec(`{
				"timestamp": "2020-11-03T21:10:25.150Z",
				"level": "INFO",
				"message": "A basic message to STDOUT"
			}`),
		}})
		event := events[0]
		assert.Equal(t, "A basic message to STDOUT", event.Data["record"])
	})

	t.Run("a null message is not a wrapped line", func(t *testing.T) {
		// Deciding the wrapper from raw bytes means asking whether the message
		// decoded into a string, and unmarshalling a JSON null into a string is
		// a no-op that reports no error. Without rejecting null explicitly this
		// would unwrap to an empty line and discard the fields beside it.
		events := postMessages(t, []LogMessage{{
			Time: "2020-11-03T21:10:25.150Z",
			Type: "function",
			Record: rec(`{
				"timestamp": "2020-11-03T21:10:25.100Z",
				"level": "INFO",
				"message": null
			}`),
		}})
		event := events[0]
		assert.NotContains(t, event.Data, "record", "a null message must not unwrap to an empty line")
		assert.Equal(t, "INFO", event.Data["level"])
		assert.Contains(t, event.Data, "timestamp")
		ts, _ := time.Parse(time.RFC3339, "2020-11-03T21:10:25.100Z")
		assert.Equal(t, ts.String(), event.Timestamp.String(),
			"the timestamp still comes from the record, not the message envelope")
	})

	t.Run("wrapped message containing JSON is unwrapped and parsed", func(t *testing.T) {
		events := postMessages(t, []LogMessage{{
			Time: epochTimestamp,
			Type: "function",
			Record: rec(`{
				"timestamp": "2020-12-25T12:34:56.789Z",
				"level": "INFO",
				"message": "{\"time\": \"2020-12-25T12:34:56.789Z\", \"samplerate\": 1, \"data\": {\"foo\": \"bar\"}}"
			}`),
		}})
		event := events[0]
		assert.Equal(t, "bar", event.Data["foo"])
	})
}
