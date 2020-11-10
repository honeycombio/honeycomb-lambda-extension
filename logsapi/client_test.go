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

	client := NewClient(server.URL, destinationPort, bufferingConfig)
	ctx := context.TODO()

	resp, err := client.Subscribe(ctx, testExtensionID)
	if err != nil {
		t.Error(err)
	}
	assert.Equal(t, "OK", resp.Message)
}
