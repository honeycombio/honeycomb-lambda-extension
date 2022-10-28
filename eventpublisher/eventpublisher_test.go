package eventpublisher

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/honeycombio/honeycomb-lambda-extension/extension"
	"github.com/stretchr/testify/assert"
)

func TestEventPublisherHappyPathSend(t *testing.T) {
	testHandler := &TestHandler{
		sleep:        0,
		responseCode: 200,
		response:     []byte(`[{"status":200}]`),
	}
	testServer := httptest.NewServer(testHandler)
	defer testServer.Close()

	testConfig := extension.Config{
		APIKey:  "test-api-key",
		Dataset: "test-dataset",
		APIHost: testServer.URL,
	}

	eventpublisherClient, err := New(testConfig, "test-version")
	assert.Nil(t, err, "unexpected error when creating client")

	err = sendTestEvent(eventpublisherClient)
	assert.Nil(t, err, "unexpected error sending test event")

	txResponse := <-eventpublisherClient.TxResponses()
	assert.Nil(t, txResponse.Err, "unexpected error in response")
	assert.Equal(t, 1, int(atomic.LoadInt64(&testHandler.callCount)), "expected a single client request")
	assert.Equal(t, 200, txResponse.StatusCode)
}

func TestEventPublisherBatchSendTimeout(t *testing.T) {
	testHandler := &TestHandler{
		sleep:        time.Millisecond * 50,
		responseCode: 200,
		response:     []byte(`[{"status":200}]`),
	}
	testServer := httptest.NewServer(testHandler)
	defer testServer.Close()

	testConfig := extension.Config{
		APIKey:           "test-api-key",
		Dataset:          "test-dataset",
		APIHost:          testServer.URL,
		BatchSendTimeout: 10 * time.Millisecond,
	}

	eventpublisherClient, err := New(testConfig, "test-version")
	assert.Nil(t, err, "unexpected error when creating client")

	err = sendTestEvent(eventpublisherClient)
	assert.Nil(t, err, "unexpected error sending test event")

	txResponse := <-eventpublisherClient.TxResponses()
	assert.Equal(t, 2, int(atomic.LoadInt64(&testHandler.callCount)), "expected 2 requests due to retry")
	assert.NotNil(t, txResponse.Err, "expected error in response")
	txResponseErr, ok := txResponse.Err.(net.Error)
	assert.True(t, ok, "expected a net.Error but got %v", txResponseErr)
	assert.True(t, txResponseErr.Timeout(), "expected error to be a timeout")
}

func TestEventPublisherConnectTimeout(t *testing.T) {
	testHandler := &TestHandler{}
	testServer := httptest.NewServer(testHandler)
	testServer.Close()

	testConfig := extension.Config{
		APIKey:         "test-api-key",
		Dataset:        "test-dataset",
		APIHost:        testServer.URL,
		ConnectTimeout: 10 * time.Millisecond,
	}

	eventpublisherClient, err := New(testConfig, "test-version")
	assert.Nil(t, err, "unexpected error creating client")

	err = sendTestEvent(eventpublisherClient)
	assert.Nil(t, err, "unexpected error sending test event")

	txResponse := <-eventpublisherClient.TxResponses()
	assert.Equal(t, 0, int(atomic.LoadInt64(&testHandler.callCount)), "expected 0 requests as server was shutdown")
	assert.NotNil(t, txResponse.Err, "expected response to be an error")
	txResponseErr, ok := txResponse.Err.(net.Error)
	assert.True(t, ok, fmt.Sprintf("expected a net.Error but got %v", txResponseErr))
	assert.ErrorIs(t, txResponseErr, syscall.ECONNREFUSED,
		fmt.Sprintf("expected connection refused error but got %v", txResponseErr))
}

func TestEventPublisherUserAgent(t *testing.T) {
	testHandler := &TestHandler{
		requestAssertions: func(r *http.Request) {
			assert.Contains(t, r.Header.Get("User-Agent"), "honeycomb-lambda-extension/a-test-version")
			assert.Contains(t, r.Header.Get("User-Agent"), runtime.GOARCH)
		},
	}
	testServer := httptest.NewServer(testHandler)
	defer testServer.Close()

	testConfig := extension.Config{
		APIKey:  "test-api-key",
		Dataset: "test-dataset",
		APIHost: testServer.URL,
	}

	eventpublisherClient, err := New(testConfig, "a-test-version")
	assert.Nil(t, err, "unexpected error when creating client")

	err = sendTestEvent(eventpublisherClient)
	assert.Nil(t, err, "unexpected error sending test event")
}

// ###########################################
// Test implementations
// ###########################################

// sendTestEvent creates a test event and flushes it
func sendTestEvent(client *Client) error {
	ev := client.NewEvent()
	ev.Add(map[string]interface{}{
		"duration_ms": 153.12,
		"method":      "test",
	})

	err := ev.Send()
	if err != nil {
		return err
	}

	client.Flush()
	return nil
}

// TestHandler is a handler used for mocking server responses for the underlying HTTP calls
// made by libhoney-go
type TestHandler struct {
	callCount         int64
	sleep             time.Duration
	requestAssertions func(r *http.Request)
	responseCode      int
	response          []byte
}

func (h *TestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&h.callCount, 1)

	if h.responseCode == 0 {
		h.responseCode = 200
	}

	if h.requestAssertions != nil {
		h.requestAssertions(r)
	}

	_, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "can't read body", http.StatusBadRequest)
		return
	}

	select {
	case <-time.After(h.sleep):
		w.WriteHeader(h.responseCode)
		w.Write(h.response)
		return
	}
}
