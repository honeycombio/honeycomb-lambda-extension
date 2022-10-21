package eventpublisher

import (
	"net"
	"net/http"
	"time"

	"github.com/honeycombio/libhoney-go"
	"github.com/honeycombio/libhoney-go/transmission"
	"github.com/sirupsen/logrus"
)

const (
	// DefaultBatchSendTimeout is the default timeout for a batch send to complete end to end.
	// This value ends up being used as the underlying libhoney-go HTTP client timeout. There
	// is a built in retry in libhoney-go which means the overall batch send timeout
	// is really 2x this value. Caution should be exercised when setting a lower value for this as
	// it's possible that a large batch may not be able to complete in the allotted time.
	DefaultBatchSendTimeout = time.Second * 15

	// DefaultConnectTimeout is the default timeout waiting for a connection to initiate. This value ends
	// up being used as the Dial timeout for the underlying libhoney-go HTTP client. This setting
	// is critical to help reduce impact caused by connectivity issues as it allows us to
	// fail fast and not have to wait for the much longer HTTP client timeout to occur
	DefaultConnectTimeout = time.Second * 3
)

var (
	log = logrus.WithFields(logrus.Fields{
		"source": "hny-lambda-ext-eventpublisher",
	})
)

// Config defines the configuration settings for Client
type Config struct {
	APIKey           string
	Dataset          string
	APIHost          string
	UserAgent        string
	BatchSendTimeout time.Duration
	ConnectTimeout   time.Duration
}

// Client is an event publisher that is just a light wrapper around libhoney
type Client struct {
	libhoneyClient *libhoney.Client
}

// New returns a configured Client
func New(config Config) (*Client, error) {
	if config.APIKey == "" || config.Dataset == "" {
		log.Warnln("APIKey or Dataset not set, disabling libhoney")
		libhoneyClient, err := libhoney.NewClient(libhoney.ClientConfig{})
		if err != nil {
			return nil, err
		}
		return &Client{libhoneyClient: libhoneyClient}, nil
	}

	batchSendTimeout := config.BatchSendTimeout
	if config.BatchSendTimeout == 0 {
		batchSendTimeout = DefaultBatchSendTimeout
	}

	connectTimeout := config.ConnectTimeout
	if config.ConnectTimeout == 0 {
		connectTimeout = DefaultConnectTimeout
	}

	// httpTransport uses settings from http.DefaultTransport as starting point, but
	// overrides the dialer connect timeout
	httpTransport := http.DefaultTransport.(*http.Transport).Clone()
	httpTransport.DialContext = (&net.Dialer{
		Timeout: connectTimeout,
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
			UserAgentAddition:     config.UserAgent,
			EnableMsgpackEncoding: true,
			BatchSendTimeout:      batchSendTimeout,
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
