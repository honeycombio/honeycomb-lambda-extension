package logsapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/honeycombio/libhoney-go/transmission"

	libhoney "github.com/honeycombio/libhoney-go"
	"github.com/stretchr/testify/assert"
)

var (
	epochTimestamp     = "1970-01-01T01:01:01.010Z"
	christmasTimestamp = "2020-12-25T12:34:56.789Z"

	platformStartMessage = LogMessage{
		Time: "2020-11-03T21:10:25.133Z",
		Type: "platform.start",
		Record: map[string]string{
			"requestId": "6d67e385-053d-4622-a56f-b25bcef23083",
			"version":   "$LATEST",
		},
	}

	nonJsonFunctionMessage = LogMessage{
		Time:   "2020-11-03T21:10:25.150Z",
		Type:   "function",
		Record: "A basic message to STDOUT",
	}

	functionMessageWithStringDurationNoTimestamp = LogMessage{
		Time:   "2020-11-03T21:10:25.150Z",
		Type:   "function",
		Record: "{\"foo\": \"bar\", \"duration_ms\": \"54\"}",
	}

	functionMessageWithIntDurationNoTimestamp = LogMessage{
		Time:   "2020-11-03T21:10:25.150Z",
		Type:   "function",
		Record: "{\"foo\": \"bar\", \"duration_ms\": 54}",
	}

	functionMessageWithFloatDurationNoTimestamp = LogMessage{
		Time:   "2020-11-03T21:10:25.150Z",
		Type:   "function",
		Record: "{\"foo\": \"bar\", \"duration_ms\": 54.43}",
	}

	functionMessageWithTimestamp = LogMessage{
		Time:   "2020-11-03T21:10:25.150Z",
		Type:   "function",
		Record: "{\"foo\": \"bar\", \"duration_ms\": 54, \"timestamp\": \"2020-11-03T21:10:25.090Z\"}",
	}

	functionMessageFromLibhoneyTransmission = LogMessage{
		Time: epochTimestamp,
		Type: "function",
		// 🎄
		Record: `{"time": "2020-12-25T12:34:56.789Z", "samplerate": 1, "data": {"foo": "bar", "duration_ms": 54} }`,
	}

	functionMessageJsonAndDataIsNotMappable = LogMessage{
		Time:   epochTimestamp,
		Type:   "function",
		Record: `{"timestamp": "2020-12-25T12:34:56.789Z", "data": "an android" }`,
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

func postMessages(t *testing.T, messages []LogMessage) []*transmission.Event {
	rr := httptest.NewRecorder()
	b, err := json.Marshal(messages)
	if err != nil {
		t.Error(err)
	}
	req, err := http.NewRequest("POST", "/", bytes.NewBuffer(b))
	if err != nil {
		t.Error(err)
	}
	testTx := &transmission.MockSender{}
	client, _ := libhoney.NewClient(libhoney.ClientConfig{
		Transmission: testTx,
		APIKey:       "blah",
	})
	handler(client).ServeHTTP(rr, req)
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
			Record: `{"time": "2020-12-25T12:34:56.789Z", "samplerate": 1, "data": {"foo": "bar", "duration_ms": 54}, "foo": "BOGUS", "duration_ms": "ALSO BOGUS" }`,
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
