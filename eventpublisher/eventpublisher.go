package eventpublisher

import (
	"fmt"
	"net"
	"net/http"
	"runtime"
	"time"

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
	if config.ApiKey == "" || config.Dataset == "" {
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
		APIKey:  config.ApiKey,
		Dataset: config.Dataset,
		APIHost: config.ApiHost,
		Transmission: &transmission.Honeycomb{
			MaxBatchSize:          libhoney.DefaultMaxBatchSize,
			BatchTimeout:          libhoney.DefaultBatchTimeout,
			MaxConcurrentBatches:  libhoney.DefaultMaxConcurrentBatches,
			PendingWorkCapacity:   libhoney.DefaultPendingWorkCapacity,
			UserAgentAddition:     fmt.Sprintf("honeycomb-lambda-extension/%s (%s)", version, runtime.GOARCH),
			EnableMsgpackEncoding: true,
			BatchSendTimeout:      config.BatchSendTimeout,
			Transport:             httpTransport,
		},
	})
	if err != nil {
		return nil, err
	}

	return &Client{
		libhoneyClient: libhoneyClient,
	}, nil
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
