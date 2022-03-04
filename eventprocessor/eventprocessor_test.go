package eventprocessor_test

import (
	"context"
	"errors"
	"testing"

	"github.com/honeycombio/honeycomb-lambda-extension/eventprocessor"
	"github.com/honeycombio/honeycomb-lambda-extension/extension"
	"github.com/honeycombio/libhoney-go"
	"github.com/honeycombio/libhoney-go/transmission"
	"github.com/stretchr/testify/assert"
)

func TestRun(t *testing.T) {
	tests := map[string]struct {
		eventPoller            *fakeEventPoller
		eventFlusher           *fakeEventFlusher
		expectedNextEventCount int
		expectedFlushCount     int
		expectedShutdownEvent  *transmission.Event
	}{
		"a single invoke event type and normal shutdown": {
			eventPoller: &fakeEventPoller{nextEventResponses: []*extension.NextEventResponse{
				{
					EventType:          extension.Invoke,
					RequestID:          "1",
					InvokedFunctionARN: "arn1",
				},
				{
					EventType:      extension.Shutdown,
					ShutdownReason: extension.ShutdownReasonSpindown,
				},
			}},
			eventFlusher:           newFakeEventFlusher(),
			expectedNextEventCount: 2,
			expectedFlushCount:     2,
			expectedShutdownEvent:  nil,
		},
		"a single invoke event type and failure shutdown": {
			eventPoller: &fakeEventPoller{nextEventResponses: []*extension.NextEventResponse{
				{
					EventType:          extension.Invoke,
					RequestID:          "1",
					InvokedFunctionARN: "arn1",
				},
				{
					EventType:      extension.Shutdown,
					ShutdownReason: extension.ShutdownReasonFailure,
				},
			}},
			eventFlusher:           newFakeEventFlusher(),
			expectedNextEventCount: 2,
			expectedFlushCount:     2,
			expectedShutdownEvent: &transmission.Event{
				Data: map[string]interface{}{
					eventprocessor.ShutdownReasonFieldExtensionType:      "platform.failure",
					eventprocessor.ShutdownReasonFieldRequestID:          "1",
					eventprocessor.ShutdownReasonFieldInvokedFunctionARN: "arn1",
				},
			},
		},
		"a single invoke event type and timeout shutdown": {
			eventPoller: &fakeEventPoller{nextEventResponses: []*extension.NextEventResponse{
				{
					EventType:          extension.Invoke,
					RequestID:          "1",
					InvokedFunctionARN: "arn1",
				},
				{
					EventType:      extension.Shutdown,
					ShutdownReason: extension.ShutdownReasonTimeout,
				},
			}},
			eventFlusher:           newFakeEventFlusher(),
			expectedNextEventCount: 2,
			expectedFlushCount:     2,
			expectedShutdownEvent: &transmission.Event{
				Data: map[string]interface{}{
					eventprocessor.ShutdownReasonFieldExtensionType:      "platform.timeout",
					eventprocessor.ShutdownReasonFieldRequestID:          "1",
					eventprocessor.ShutdownReasonFieldInvokedFunctionARN: "arn1",
				},
			},
		},
		"a timeout shutdown (no last requestId)": {
			eventPoller: &fakeEventPoller{nextEventResponses: []*extension.NextEventResponse{
				{
					EventType:      extension.Shutdown,
					ShutdownReason: extension.ShutdownReasonTimeout,
				},
			}},
			eventFlusher:           newFakeEventFlusher(),
			expectedNextEventCount: 1,
			expectedFlushCount:     1,
			expectedShutdownEvent:  nil,
		},
		"a single next event error, then normal invoke event type and normal shutdown": {
			eventPoller: &fakeEventPoller{
				oneTimeError: errors.New("one time error"),
				nextEventResponses: []*extension.NextEventResponse{
					{
						EventType:          extension.Invoke,
						RequestID:          "1",
						InvokedFunctionARN: "arn1",
					},
					{
						EventType:      extension.Shutdown,
						ShutdownReason: extension.ShutdownReasonSpindown,
					},
				},
			},
			eventFlusher:           newFakeEventFlusher(),
			expectedNextEventCount: 2,
			expectedFlushCount:     2,
			expectedShutdownEvent:  nil,
		},
		"an unknown event type and normal shutdown": {
			eventPoller: &fakeEventPoller{
				nextEventResponses: []*extension.NextEventResponse{
					{
						EventType: "unknown",
					},
					{
						EventType:      extension.Shutdown,
						ShutdownReason: extension.ShutdownReasonSpindown,
					},
				},
			},
			eventFlusher:           newFakeEventFlusher(),
			expectedNextEventCount: 2,
			expectedFlushCount:     2,
			expectedShutdownEvent:  nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			processor := eventprocessor.New(tc.eventPoller, tc.eventFlusher)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			processor.Run(ctx, cancel)

			assert.Equal(t, tc.expectedNextEventCount, tc.eventPoller.nextEventCounter, "next event calls do not match")
			assert.Equal(t, tc.expectedFlushCount, tc.eventFlusher.mockSender.Flushed, "flush calls do not match")
			if tc.expectedShutdownEvent != nil {
				assert.Equal(t, tc.expectedShutdownEvent.Data, tc.eventFlusher.mockSender.Events()[0].Data, "shutdown event does not match")
			}
		})
	}
}

// ###########################################
// Test implementations
// ###########################################

type fakeEventPoller struct {
	oneTimeError       error
	nextEventCounter   int
	nextEventResponses []*extension.NextEventResponse
}

func (f *fakeEventPoller) NextEvent(ctx context.Context) (*extension.NextEventResponse, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if f.oneTimeError != nil {
		err := f.oneTimeError
		f.oneTimeError = nil
		return nil, err
	}
	resp := f.nextEventResponses[f.nextEventCounter]
	f.nextEventCounter++
	return resp, nil
}

func newFakeEventFlusher() *fakeEventFlusher {
	mockSender := &transmission.MockSender{}
	libhoneyClient, _ := libhoney.NewClient(libhoney.ClientConfig{
		APIKey:       "unit-test-api-key",
		Dataset:      "unit-test-dataset",
		Transmission: mockSender,
	})
	return &fakeEventFlusher{
		mockSender:     mockSender,
		libhoneyClient: libhoneyClient,
	}
}

type fakeEventFlusher struct {
	libhoneyClient *libhoney.Client
	mockSender     *transmission.MockSender
}

func (f *fakeEventFlusher) NewEvent() *libhoney.Event {
	return f.libhoneyClient.NewEvent()
}

func (f *fakeEventFlusher) Flush() {
	f.mockSender.Flush()
}
