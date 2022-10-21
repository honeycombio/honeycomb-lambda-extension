package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"path/filepath"
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
	exit := make(chan os.Signal, 1)
	signal.Notify(exit, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	go func() {
		sig := <-exit
		log.Warn("Received ", sig, " - shutting down.")
		cancel()
	}()

	// initialize event publisher client
	eventpublisherClient, err := eventpublisher.New(config, version)
	if err != nil {
		log.Warn("Could not initialize event publisher", err)
	}

	// initialize Logs API HTTP server
	go logsapi.StartHTTPServer(config.LogsReceiverPort, eventpublisherClient)

	// if running in localMode, wait on the context to be cancelled,
	// then early return main() to end the process
	if localMode {
		select {
		case <-ctx.Done():
			return
		}
	}

	// --- Lambda Runtime Activity ---

	// register with Extensions API
	extensionClient := extension.NewClient(config.RuntimeAPI, extensionName)
	res, err := extensionClient.Register(ctx)
	if err != nil {
		log.Panic("Could not register extension", err)
	}
	log.Debug("Response from register: ", res)

	// create logs api client
	logsClient := logsapi.NewClient(config.RuntimeAPI, config.LogsReceiverPort, logsapi.BufferingOptions{
		TimeoutMS: uint(config.LogsAPITimeoutMS),
		MaxBytes:  uint64(config.LogsAPIMaxBytes),
		MaxItems:  uint64(config.LogsAPIMaxItems),
	})

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
