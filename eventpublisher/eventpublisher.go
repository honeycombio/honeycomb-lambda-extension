package eventpublisher

import (
	"fmt"
	"net"
	"net/http"

	"github.com/honeycombio/honeycomb-lambda-extension/extension"
	"github.com/honeycombio/libhoney-go"
	"github.com/honeycombio/libhoney-go/transmission"
	"github.com/sirupsen/logrus"
)

var (
	log = logrus.WithFields(logrus.Fields{
		"source": "hny-lambda-ext-eventpublisher",
	})
)

// Client is an event publisher that is just a light wrapper around libhoney
type Client struct {
	libhoneyClient *libhoney.Client
}

// New returns a configured Client
func New(config extension.Config, version string) (*Client, error) {
	if config.APIKey == "" || config.Dataset == "" {
		log.Warnln("APIKey or Dataset not set, disabling libhoney")
		libhoneyClient, err := libhoney.NewClient(libhoney.ClientConfig{})
		if err != nil {
			return nil, err
		}
		return &Client{libhoneyClient: libhoneyClient}, nil
	}

	// httpTransport uses settings from http.DefaultTransport as starting point, but
	// overrides the dialer connect timeout
	httpTransport := http.DefaultTransport.(*http.Transport).Clone()
	httpTransport.DialContext = (&net.Dialer{
		Timeout: config.ConnectTimeout,
	}).DialContext

	libhoneyClient, err := libhoney.NewClient(libhoney.ClientConfig{
		APIKey:  config.APIKey,
		Dataset: config.Dataset,
		APIHost: config.APIHost,
		Transmission: &transmission.Honeycomb{
			MaxBatchSize:          libhoney.DefaultMaxBatchSize,
			BatchTimeout:          libhoney.DefaultBatchTimeout,
			MaxConcurrentBatches:  libhoney.DefaultMaxConcurrentBatches,
			PendingWorkCapacity:   libhoney.DefaultPendingWorkCapacity,
			UserAgentAddition:     fmt.Sprintf("honeycomb-lambda-extension/%s", version),
			EnableMsgpackEncoding: true,
			BatchSendTimeout:      config.BatchSendTimeout,
			Transport:             httpTransport,
		},
	})
	if err != nil {
		return nil, err
	}

	publisher := &Client{libhoneyClient: libhoneyClient}

	if config.Debug {
		go publisher.readResponses()
	}

	return publisher, nil
}

func (c *Client) NewEvent() *libhoney.Event {
	return c.libhoneyClient.NewEvent()
}

func (c *Client) Flush() {
	c.libhoneyClient.Flush()
}

func (c *Client) TxResponses() chan transmission.Response {
	return c.libhoneyClient.TxResponses()
}

// read batch send responses from Honeycomb and log success/failures
func (c *Client) readResponses() {
	for r := range c.TxResponses() {
		var metadata string
		if r.Metadata != nil {
			metadata = fmt.Sprintf("%s", r.Metadata)
		}
		if r.StatusCode >= 200 && r.StatusCode < 300 {
			message := "Successfully sent event to Honeycomb"
			if metadata != "" {
				message += fmt.Sprintf(": %s", metadata)
			}
			log.Debugf("%s", message)
		} else if r.StatusCode == http.StatusUnauthorized {
			log.Debugf("Error sending event to honeycomb! The APIKey was rejected, please verify your APIKey. %s", metadata)
		} else {
			log.Debugf("Error sending event to Honeycomb! %s had code %d, err %v and response body %s",
				metadata, r.StatusCode, r.Err, r.Body)
		}
	}
}
