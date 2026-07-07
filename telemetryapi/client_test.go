package telemetryapi

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	destinationPort = 3000
	bufferingConfig = BufferingOptions{
		TimeoutMS: 1000,
		MaxBytes:  262144,
		MaxItems:  1000,
	}
	testExtensionID = "extensionID"
)

func SubscribeServer(t *testing.T) (*httptest.Server, *SubscribeRequest) {
	received := &SubscribeRequest{}
	handlerFunc := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if err := json.Unmarshal(body, received); err != nil {
			t.Error(err)
		}
		resp := SubscribeResponse{
			Message: "OK",
		}
		w.Write([]byte(resp.Message))
	})

	return httptest.NewServer(handlerFunc), received
}

func TestSubscribeTelemetry(t *testing.T) {
	server, received := SubscribeServer(t)
	defer server.Close()

	client := newClient(server.URL, destinationPort, bufferingConfig)
	ctx := context.TODO()

	resp, err := client.subscribeToTelemetryTypes(ctx, testExtensionID, []LogType{PlatformLog, FunctionLog})
	if err != nil {
		t.Error(err)
	}
	assert.Equal(t, "OK", resp.Message)
	assert.Equal(t, schemaVersion, received.SchemaVersion)
	assert.Equal(t, []LogType{PlatformLog, FunctionLog}, received.Types)
}

func TestURL(t *testing.T) {
	client := newClient("honeycomb.io/foo", 3000, BufferingOptions{})
	assert.Equal(t, "http://honeycomb.io/foo/2022-07-01", client.baseURL)

	url := client.url("/foo/bar/baz")
	assert.Equal(t, "http://honeycomb.io/foo/2022-07-01/foo/bar/baz", url)

	client = newClient("https://mywebsite.com:9000", 3000, BufferingOptions{})

	assert.Equal(t, "https://mywebsite.com:9000/2022-07-01", client.baseURL)
	assert.Equal(t, "https://mywebsite.com:9000/2022-07-01/foo/bar", client.url("foo/bar"))
}
