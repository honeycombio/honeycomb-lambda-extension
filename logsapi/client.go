package logsapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strings"

	log "github.com/sirupsen/logrus"
)

// Protocol represents the protocol that this extension should receive logs by
type Protocol string

// LogType represents the types of log messages that are supported by the logs API
type LogType string

const (
	// HTTPProtocol is the protocol that we receive logs over
	HTTPProtocol Protocol = "HTTP"

	// PlatformLog events originate from the Lambda Runtime
	PlatformLog LogType = "platform"
	// FunctionLog events originate from Lambda Functions
	FunctionLog LogType = "function"

	// extensionIdentifierHeader is used to pass a generated UUID to calls to the API
	extensionIdentifierHeader = "Lambda-Extension-Identifier"
)

// Destination is where the runtime should send logs to
type Destination struct {
	Protocol Protocol `json:"protocol"`
	URI      string   `json:"URI"`
}

// BufferingOptions contains buffering configuration options for the lambda platform
type BufferingOptions struct {
	TimeoutMS uint   `json:"timeoutMs"`
	MaxBytes  uint64 `json:"maxBytes"`
	MaxItems  uint64 `json:"maxItems"`
}

// Client is used to communicate with the Logs API
type Client struct {
	baseURL          string
	httpClient       *http.Client
	destinationPort  int
	bufferingOptions BufferingOptions
	ExtensionID      string
}

// SubscribeRequest is the request to /logs
type SubscribeRequest struct {
	Dest      Destination      `json:"destination"`
	Types     []LogType        `json:"types"`
	Buffering BufferingOptions `json:"buffering"`
}

// SubscribeResponse is the response from /logs subscribe message
type SubscribeResponse struct {
	Message string
}

// NewClient returns a new Lambda Logs API client
func NewClient(baseURL string, port int, bufferingOpts BufferingOptions) *Client {
	if !strings.HasPrefix(baseURL, "http") {
		baseURL = fmt.Sprintf("http://%s", baseURL)
	}
	baseURL = fmt.Sprintf("%s/2020-08-15", baseURL)
	return &Client{
		baseURL:          baseURL,
		httpClient:       &http.Client{},
		destinationPort:  port,
		bufferingOptions: bufferingOpts,
	}
}

// Subscribe will subscribe to events sent from the Logs API
func (c *Client) Subscribe(ctx context.Context, extensionID string, types []LogType) (*SubscribeResponse, error) {
	subscribe := SubscribeRequest{
		Dest: Destination{
			Protocol: HTTPProtocol,
			URI:      fmt.Sprintf("http://sandbox:%d", c.destinationPort),
		},
		Types:     types,
		Buffering: c.bufferingOptions,
	}
	reqBody, err := json.Marshal(subscribe)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, "PUT", c.url("/logs"), bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set(extensionIdentifierHeader, extensionID)
	httpRes, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if httpRes.StatusCode != 200 {
		return nil, fmt.Errorf("request failed with status %s", httpRes.Status)
	}
	defer httpRes.Body.Close()
	body, err := ioutil.ReadAll(httpRes.Body)
	if err != nil {
		return nil, err
	}
	c.ExtensionID = httpRes.Header.Get(extensionIdentifierHeader)
	if len(c.ExtensionID) == 0 {
		log.Warn("No extension identifier returned in header")
	}
	return &SubscribeResponse{
		Message: string(body),
	}, nil
}

// url is a helper function to build urls out of relative paths
func (c *Client) url(requestPath string) string {
	newURL, err := url.Parse(c.baseURL)
	if err != nil {
		return fmt.Sprintf("%s%s", c.baseURL, requestPath)
	}
	newURL.Path = path.Join(newURL.Path, requestPath)
	return newURL.String()
}
