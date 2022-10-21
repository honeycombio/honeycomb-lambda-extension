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
	version string           // Fed in at build with -ldflags "-X main.version=<value>"
	config  extension.Config // Honeycomb extension configuration

	// extension API configuration
	extensionName = filepath.Base(os.Args[0])

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

	config = extension.NewConfigFromEnvironment()

	logLevel := logrus.InfoLevel
	if config.Debug {
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
	extensionClient := extension.NewClient(config.RuntimeAPI, extensionName)
	if !localMode {
		res, err := extensionClient.Register(ctx)
		if err != nil {
			log.Panic("Could not register extension", err)
		}
		log.Debug("Response from register: ", res)
	}

	// initialize event publisher client
	eventpublisherClient, err := eventpublisher.New(eventpublisher.Config{
		APIKey:           config.ApiKey,
		Dataset:          config.Dataset,
		APIHost:          config.ApiHost,
		BatchSendTimeout: config.BatchSendTimeout,
		ConnectTimeout:   config.ConnectTimeout,
		UserAgent:        fmt.Sprintf("honeycomb-lambda-extension/%s (%s)", version, runtime.GOARCH),
	})
	if config.Debug {
		go readResponses(eventpublisherClient)
	}
	if err != nil {
		log.Warn("Could not initialize libhoney", err)
	}

	// initialize Logs API HTTP server
	go logsapi.StartHTTPServer(config.LogsReceiverPort, eventpublisherClient)

	// create logs api client
	logsClient := logsapi.NewClient(config.RuntimeAPI, config.LogsReceiverPort, logsapi.BufferingOptions{
		TimeoutMS: uint(config.LogsAPITimeoutMS),
		MaxBytes:  uint64(config.LogsAPIMaxBytes),
		MaxItems:  uint64(config.LogsAPIMaxItems),
	})

	// if running in localMode, just wait on the context to be cancelled
	if localMode {
		select {
		case <-ctx.Done():
			return
		}
	}

	var logTypes []logsapi.LogType
	disablePlatformMsg := config.LogsAPIDisablePlatformMessages

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
