package logsapi

import (
	"context"
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

func SubscribeServer(t *testing.T) *httptest.Server {
	handlerFunc := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := SubscribeResponse{
			Message: "OK",
		}
		w.Write([]byte(resp.Message))
	})

	return httptest.NewServer(handlerFunc)
}

func TestSubscribeLogs(t *testing.T) {
	server := SubscribeServer(t)
	defer server.Close()

	client := newClient(server.URL, destinationPort, bufferingConfig)
	ctx := context.TODO()

	resp, err := client.subscribeToLogTypes(ctx, testExtensionID, []LogType{PlatformLog, FunctionLog})
	if err != nil {
		t.Error(err)
	}
	assert.Equal(t, "OK", resp.Message)
}

func TestURL(t *testing.T) {
	client := newClient("honeycomb.io/foo", 3000, BufferingOptions{})
	assert.Equal(t, "http://honeycomb.io/foo/2020-08-15", client.baseURL)

	url := client.url("/foo/bar/baz")
	assert.Equal(t, "http://honeycomb.io/foo/2020-08-15/foo/bar/baz", url)

	client = newClient("https://mywebsite.com:9000", 3000, BufferingOptions{})

	assert.Equal(t, "https://mywebsite.com:9000/2020-08-15", client.baseURL)
	assert.Equal(t, "https://mywebsite.com:9000/2020-08-15/foo/bar", client.url("foo/bar"))
}
