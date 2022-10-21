package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	logrus "github.com/sirupsen/logrus"

	"github.com/honeycombio/honeycomb-lambda-extension/config"
	"github.com/honeycombio/honeycomb-lambda-extension/eventprocessor"
	"github.com/honeycombio/honeycomb-lambda-extension/eventpublisher"
	"github.com/honeycombio/honeycomb-lambda-extension/extension"
	"github.com/honeycombio/honeycomb-lambda-extension/logsapi"
)

var (
	version   string        // Fed in at build with -ldflags "-X main.version=<value>"
	extConfig config.Config // Honeycomb extension configuration

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

	extConfig = config.FromEnvironment()

	logLevel := logrus.InfoLevel
	if extConfig.Debug {
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
		s := <-exit
		log.Warn("Received ", s, " - Exiting")
		cancel()
	}()

	// initialize event publisher client
	eventpublisherClient, err := eventpublisher.New(extConfig, version)

	if err != nil {
		log.Warn("Could not initialize client for publishing events to Honeycomb", err)
	}

	// initialize the extension's log receiver
	go logsapi.StartLogsReceiver(extConfig.LogsReceiverPort, eventpublisherClient)

	// if running in localMode, just wait on the context to be cancelled
	if localMode {
		// ... wait on the context to the cancelled, then return from main
		select {
		case <-ctx.Done():
			return
		}
	}

	// ### not localMode ###

	// register with Extensions API
	extensionClient := extension.NewClient(extConfig.RuntimeAPI, extensionName)
	res, err := extensionClient.Register(ctx)
	if err != nil {
		log.Panic("Could not register extension", err)
	}
	log.Debug("Response from register: ", res)

	// subscribe to log output from the lambda
	subscription, err := logsapi.FriendlierSubscribe(ctx, extConfig, extensionClient.ExtensionID)
	if err != nil {
		log.Warn("Could not subscribe to events: ", err)
	}
	log.Debug("Response from subscribe: ", subscription)

	eventprocessor.New(extensionClient, eventpublisherClient).Run(ctx, cancel)
}
