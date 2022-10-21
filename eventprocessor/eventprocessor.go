package eventprocessor

import (
	"context"
	"fmt"

	"github.com/honeycombio/honeycomb-lambda-extension/extension"
	"github.com/honeycombio/libhoney-go"
	"github.com/sirupsen/logrus"
)

var (
	log = logrus.WithFields(logrus.Fields{
		"source": "hny-lambda-ext-eventprocessor",
	})
	// ShutdownReasonFieldExtensionType is the field name for shutdown reason in shutdown reason event
	ShutdownReasonFieldExtensionType = "lambda_extension.type"
	// ShutdownReasonFieldRequestID is the field name used for request ID in shutdown reason event
	ShutdownReasonFieldRequestID = "requestId"
	// ShutdownReasonFieldInvokedFunctionARN is the field name used for function arn in shutdown reason event
	ShutdownReasonFieldInvokedFunctionARN = "invokedFunctionArn"
)

// eventPoller is the interface that provides a next event for the event processor
type eventPoller interface {
	NextEvent(ctx context.Context) (*extension.NextEventResponse, error)
}

// eventFlusher is the interface that provides a way to create new libhoney events and flush them
type eventFlusher interface {
	NewEvent() *libhoney.Event
	Flush()
}

// Server represents a server that polls and processes Lambda extension events
type Server struct {
	extensionClient    eventPoller
	libhoneyClient     eventFlusher
	invokedFunctionARN string
	lastRequestId      string
}

// New takes an eventPoller and eventFlusher and returns a Server
func New(extensionClient eventPoller, libhoneyClient eventFlusher) *Server {
	return &Server{
		extensionClient: extensionClient,
		libhoneyClient:  libhoneyClient,
	}
}

// Run executes an event loop to poll and process events from the Lambda extension API
func (s *Server) Run(ctx context.Context, cancel context.CancelFunc) {
	log.Debug("Starting ...")
	for {
		select {
		case <-ctx.Done():
			return
		default:
			s.pollEventAndProcess(ctx, cancel)
		}
	}
}

// pollEventAndProcess polls the Lambda extension next event API and processes a single event
func (s *Server) pollEventAndProcess(ctx context.Context, cancel context.CancelFunc) {
	// Poll for event
	res, err := s.extensionClient.NextEvent(ctx)
	if err != nil {
		log.WithError(err).Warn("Error from NextEvent")
		return
	}

	// Ensure a flush happens and cancel is called if its a shutdown event
	defer func() {
		s.libhoneyClient.Flush()
		if res.EventType == extension.Shutdown {
			cancel()
		}
	}()

	// Handles event types
	switch eventType := res.EventType; eventType {
	case extension.Invoke:
		log.Debug("Received INVOKE event.")
		s.lastRequestId = res.RequestID
		s.invokedFunctionARN = res.InvokedFunctionARN
	case extension.Shutdown:
		log.Debug("Received SHUTDOWN event.")
		if res.ShutdownReason != extension.ShutdownReasonSpindown && s.lastRequestId != "" {
			log.WithField("res.ShutdownReason", res.ShutdownReason).Debug("Sending shutdown reason")
			s.sendShutdownReason(res.ShutdownReason)
		}
	default:
		log.WithField("res", res).Debug("Received unknown event")
	}
}

// sendShutdownReason sends and flushes an event with the shutdown reason. The last
// request ID and function ARN will also be include in the generated event.
func (s *Server) sendShutdownReason(shutdownReason extension.ShutdownReason) {
	ev := s.libhoneyClient.NewEvent()
	ev.AddField(ShutdownReasonFieldExtensionType, fmt.Sprintf("platform.%s", shutdownReason))
	ev.AddField(ShutdownReasonFieldRequestID, s.lastRequestId)
	ev.AddField(ShutdownReasonFieldInvokedFunctionARN, s.invokedFunctionARN)
	if err := ev.Send(); err != nil {
		log.WithError(err).Error("Unable to send event with shutdown reason")
	}
}
