package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	libhoney "github.com/honeycombio/libhoney-go"
	logrus "github.com/sirupsen/logrus"

	"github.com/honeycombio/honeycomb-lambda-extension/extension"
	"github.com/honeycombio/honeycomb-lambda-extension/logsapi"
)

var (
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

	// when run in local mode, we don't attempt to register the extension or subscribe
	// to log events - useful for testing
	localMode = false

	// set up logging defaults for our own logging output
	log = logrus.WithFields(logrus.Fields{
		"source": "hny-lambda-ext-main",
	})
)

func init() {
	envLogLevel, ok := os.LookupEnv("HNY_LOG_LEVEL")
	if !ok {
		envLogLevel = "info"
	}
	parsedLogLevel, err := logrus.ParseLevel(envLogLevel)
	if err != nil {
		parsedLogLevel = logrus.InfoLevel
	}
	logrus.SetLevel(parsedLogLevel)
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

	// initialize libhoney
	libhoney.UserAgentAddition = fmt.Sprintf("honeycomb-lambda-extension/%s", version)
	client, err := libhoney.NewClient(libhoneyConfig())
	if err != nil {
		log.Warn("Could not initialize libhoney", err)
	}

	// off-the-cuff Honey API response logger
	go func() {
		response := <-client.TxResponses()
		log.Debug("Libhoney TxResponse: ", response.StatusCode)
	}()

	// initialize Logs API HTTP server
	go logsapi.StartHTTPServer(logsServerPort, client)

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

	// poll the extension API for the next (invoke or shutdown) event
	for {
		select {
		case <-ctx.Done():
			return
		default:
			res, err := extensionClient.NextEvent(ctx)
			if err != nil {
				log.Warn("Error from NextEvent: ", err)
				continue
			}
			// Flush events queue after waking up
			client.Flush()
			if res.EventType == extension.Shutdown {
				log.Debug("Received SHUTDOWN event. Exiting.")
				cancel()
			} else if res.EventType == extension.Invoke {
				log.Debug("Received INVOKE event.")
			} else {
				log.Debug("Received unknown event: ", res)
			}
		}
	}
}

// configure libhoney
func libhoneyConfig() libhoney.ClientConfig {
	if apiKey == "" || dataset == "" {
		log.Warnln("LIBHONEY_API_KEY or LIBHONEY_DATASET not set, disabling libhoney")
		return libhoney.ClientConfig{}
	}

	var libhoneyLogger libhoney.Logger
	currentLogLevel := logrus.GetLevel()
	if currentLogLevel == logrus.DebugLevel || currentLogLevel == logrus.TraceLevel {
		libhoneyLogger = logrus.WithFields(logrus.Fields{
			"source": "hny-lambda-ext-libhoney",
		})
	} else {
		libhoneyLogger = nil
	}

	return libhoney.ClientConfig{
		APIKey:  apiKey,
		Dataset: dataset,
		APIHost: apiHost,
		Logger:  libhoneyLogger,
	}
}

// helper functions for environment variables with default fallbacks
func envOrElseInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		v, err := strconv.Atoi(value)
		if err != nil {
			return fallback
		}
		return v
	}
	return fallback
}

func envOrElseBool(key string, fallback bool) bool {
	if value, ok := os.LookupEnv(key); ok {
		v, err := strconv.ParseBool(value)
		if err != nil {
			return fallback
		}
		return v
	}
	return fallback
}
