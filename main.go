package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	logrus "github.com/sirupsen/logrus"

	"github.com/honeycombio/honeycomb-lambda-extension/eventprocessor"
	"github.com/honeycombio/honeycomb-lambda-extension/eventpublisher"
	"github.com/honeycombio/honeycomb-lambda-extension/extension"
	"github.com/honeycombio/honeycomb-lambda-extension/logsapi"
)

var (
	version string // Fed in at build with -ldflags "-X main.version=<value>"

	// This environment variable is set in the extension environment. It's expected to be
	// a hostname:port combination.
	runtimeAPI = os.Getenv("AWS_LAMBDA_RUNTIME_API")

	// extension API configuration
	extensionName   = filepath.Base(os.Args[0])
	extensionClient = extension.NewClient(runtimeAPI, extensionName)

	// logs API configuration
	logsServerPort = 3000

	// default buffering options for logs api
	defaultTimeoutMS = 1000
	defaultMaxBytes  = 262144
	defaultMaxItems  = 1000

	// honeycomb configuration
	apiKey  = os.Getenv("LIBHONEY_API_KEY")
	dataset = os.Getenv("LIBHONEY_DATASET")
	apiHost = os.Getenv("LIBHONEY_API_HOST")
	debug   = envOrElseBool("HONEYCOMB_DEBUG", false)

	// when run in local mode, we don't attempt to register the extension or subscribe
	// to log events - useful for testing
	localMode = false

	// set up logging defaults for our own logging output
	log = logrus.WithFields(logrus.Fields{
		"source": "hny-lambda-ext-main",
	})
)

func init() {
	if version == "" {
		version = "dev"
	}

	logLevel := logrus.InfoLevel
	if debug {
		logLevel = logrus.DebugLevel
	}
	logrus.SetLevel(logLevel)
}

func main() {
	flag.BoolVar(&localMode, "localMode", false, "do not attempt to register or subscribe")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())

	// exit cleanly on SIGTERM or SIGINT
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		s := <-sigs
		cancel()
		log.Warn("Received", s, "Exiting")
	}()

	// register with Extensions API
	if !localMode {
		res, err := extensionClient.Register(ctx)
		if err != nil {
			log.Panic("Could not register extension", err)
		}
		log.Debug("Response from register: ", res)
	}

	// initialize event publisher client
	eventpublisherClient, err := eventpublisher.New(eventpublisher.Config{
		APIKey:           apiKey,
		Dataset:          dataset,
		APIHost:          apiHost,
		BatchSendTimeout: envOrElseDuration("HONEYCOMB_BATCH_SEND_TIMEOUT", eventpublisher.DefaultBatchSendTimeout),
		ConnectTimeout:   envOrElseDuration("HONEYCOMB_CONNECT_TIMEOUT", eventpublisher.DefaultConnectTimeout),
		UserAgent:        fmt.Sprintf("honeycomb-lambda-extension/%s (%s)", version, runtime.GOARCH),
	})
	if debug {
		go readResponses(eventpublisherClient)
	}
	if err != nil {
		log.Warn("Could not initialize libhoney", err)
	}

	// initialize Logs API HTTP server
	go logsapi.StartHTTPServer(logsServerPort, eventpublisherClient)

	// create logs api client
	logsClient := logsapi.NewClient(runtimeAPI, logsServerPort, logsapi.BufferingOptions{
		TimeoutMS: uint(envOrElseInt("LOGS_API_TIMEOUT_MS", defaultTimeoutMS)),
		MaxBytes:  uint64(envOrElseInt("LOGS_API_MAX_BYTES", defaultMaxBytes)),
		MaxItems:  uint64(envOrElseInt("LOGS_API_MAX_ITEMS", defaultMaxItems)),
	})

	// if running in localMode, just wait on the context to be cancelled
	if localMode {
		select {
		case <-ctx.Done():
			return
		}
	}

	var logTypes []logsapi.LogType
	disablePlatformMsg := envOrElseBool("LOGS_API_DISABLE_PLATFORM_MSGS", false)

	if disablePlatformMsg {
		logTypes = []logsapi.LogType{logsapi.FunctionLog}
	} else {
		logTypes = []logsapi.LogType{logsapi.PlatformLog, logsapi.FunctionLog}
	}

	subRes, err := logsClient.Subscribe(ctx, extensionClient.ExtensionID, logTypes)
	if err != nil {
		log.Warn("Could not subscribe to events: ", err)
	}
	log.Debug("Response from subscribe: ", subRes)

	eventprocessor.New(extensionClient, eventpublisherClient).Run(ctx, cancel)
}

func readResponses(client *eventpublisher.Client) {
	for r := range client.TxResponses() {
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
