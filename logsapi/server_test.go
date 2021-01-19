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

func getLogMessages() []LogMessage {
	return []LogMessage{
		{
			Time: "2020-11-03T21:10:25.133Z",
			Type: "platform.start",
			Record: map[string]string{
				"requestId": "6d67e385-053d-4622-a56f-b25bcef23083",
				"version":   "$LATEST",
				"timestamp": "2020-11-03T21:10:25.150Z",
			},
		},
		{
			Time:   "2020-11-03T21:10:25.150Z",
			Type:   "function",
			Record: "A basic message to STDOUT",
		},
		{
			Time:   "2020-11-03T21:10:25.150Z",
			Type:   "function",
			Record: "{\"foo\": \"bar\", \"duration_ms\": \"54ms\"}",
		},
		{
			Time:   "2020-11-03T21:10:25.150Z",
			Type:   "function",
			Record: "{\"foo\": \"bar\", \"duration_ms\": \"54ms\", \"timestamp\": \"2020-11-03T21:10:25.090Z\"}",
		},
	}
}

func TestLogMessage(t *testing.T) {
	rr := httptest.NewRecorder()
	b, err := json.Marshal(getLogMessages())
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

	assert.Equal(t, 4, len(testTx.Events()))
	assert.Equal(t, "platform.start", testTx.Events()[0].Data["lambda_extension.type"])
	assert.Equal(t, "$LATEST", testTx.Events()[0].Data["version"])
	assert.Equal(t, "bar", testTx.Events()[2].Data["foo"])

	// try to parse the timestamp from the event body of a platform message
	ts, err := time.Parse(time.RFC3339, "2020-11-03T21:10:25.150Z")
	if err != nil {
		assert.Fail(t, "Could not parse timestamp")
	}
	assert.Equal(t, ts.String(), testTx.Events()[0].Timestamp.String())

	// try to parse the timestamp from the event body of a function message
	ts, err = time.Parse(time.RFC3339, "2020-11-03T21:10:25.090Z")
	if err != nil {
		assert.Fail(t, "Could not parse timestamp")
	}
	assert.Equal(t, ts.String(), testTx.Events()[3].Timestamp.String())

	// when no timestamp is present in the body, take the event timestamp and subtract duration
	ts, err = time.Parse(time.RFC3339, "2020-11-03T21:10:25.150Z")
	if err != nil {
		assert.Fail(t, "Could not parse timestamp")
	}
	duration := fmt.Sprintf("%s", testTx.Events()[2].Data["duration_ms"])
	d, err := time.ParseDuration(duration)
	if err != nil {
		assert.Fail(t, "Could not parse duration")
	}
	ts = ts.Add(-1 * d)
	assert.Equal(t, ts.String(), testTx.Events()[2].Timestamp.String())
}
