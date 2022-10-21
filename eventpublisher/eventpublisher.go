package eventpublisher

import (
	"fmt"
	"net"
	"net/http"
	"runtime"

	"github.com/honeycombio/honeycomb-lambda-extension/config"
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

// New returns a configured eventpublisher.Client
func New(cfg config.Config, version string) (*Client, error) {
	if cfg.ApiKey == "" || cfg.Dataset == "" {
		log.Warnln("APIKey or Dataset not set, disabling libhoney")
		libhoneyClient, err := libhoney.NewClient(libhoney.ClientConfig{})
		if err != nil {
			return nil, err
		}
		return &Client{libhoneyClient: libhoneyClient}, nil
	}

	userAgentAddition := fmt.Sprintf("honeycomb-lambda-extension/%s (%s)", version, runtime.GOARCH)

	// httpTransport uses settings from http.DefaultTransport as starting point,
	// then applies some configuration
	httpTransport := http.DefaultTransport.(*http.Transport).Clone()
	httpTransport.DialContext = (&net.Dialer{
		Timeout: cfg.ConnectTimeout,
	}).DialContext

	libhoneyClient, err := libhoney.NewClient(libhoney.ClientConfig{
		APIKey:  cfg.ApiKey,
		Dataset: cfg.Dataset,
		APIHost: cfg.ApiHost,
		Transmission: &transmission.Honeycomb{
			MaxBatchSize:          libhoney.DefaultMaxBatchSize,
			BatchTimeout:          libhoney.DefaultBatchTimeout,
			MaxConcurrentBatches:  libhoney.DefaultMaxConcurrentBatches,
			PendingWorkCapacity:   libhoney.DefaultPendingWorkCapacity,
			UserAgentAddition:     userAgentAddition,
			EnableMsgpackEncoding: true,
			BatchSendTimeout:      cfg.BatchSendTimeout,
			Transport:             httpTransport,
		},
	})
	if err != nil {
		return nil, err
	}

	eventpublisherClient := &Client{libhoneyClient: libhoneyClient}

	if cfg.Debug {
		go eventpublisherClient.readResponses()
	}

	return eventpublisherClient, nil
}

func (c *Client) NewEvent() *libhoney.Event {
	return c.libhoneyClient.NewEvent()
}

func (c *Client) Flush() {
	c.libhoneyClient.Flush()
}

func (c *Client) txResponses() chan transmission.Response {
	return c.libhoneyClient.TxResponses()
}

func (c *Client) readResponses() {
	for r := range c.libhoneyClient.TxResponses() {
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
