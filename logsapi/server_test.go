package logsapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
			Record: "{\"foo\": \"bar\"}",
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

	assert.Equal(t, 3, len(testTx.Events()))
	assert.Equal(t, "platform.start", testTx.Events()[0].Data["lambda_extension.type"])
	assert.Equal(t, "$LATEST", testTx.Events()[0].Data["version"])
	assert.Equal(t, "bar", testTx.Events()[2].Data["foo"])
}
