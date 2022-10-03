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
	"strconv"
	"syscall"
	"time"

	logrus "github.com/sirupsen/logrus"

	"github.com/honeycombio/honeycomb-lambda-extension/eventprocessor"
	"github.com/honeycombio/honeycomb-lambda-extension/extension"
	"github.com/honeycombio/honeycomb-lambda-extension/logsapi"
	libhoney "github.com/honeycombio/libhoney-go"
	"github.com/honeycombio/libhoney-go/transmission"
)

const (
	// Waiting too long to send a batch of events can be
	// expensive in Lambda. It's reasonable to expect a
	// batch send to complete in this amount of time.
	defaultBatchSendTimeout = time.Second * 15
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

	// initialize libhoney
	libhoneyClient, err := libhoney.NewClient(libhoneyConfig())
	if debug {
		go readResponses(libhoneyClient.TxResponses())
	}
	if err != nil {
		log.Warn("Could not initialize libhoney", err)
	}

	// initialize Logs API HTTP server
	go logsapi.StartHTTPServer(logsServerPort, libhoneyClient)

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

	eventprocessor.New(extensionClient, libhoneyClient).Run(ctx, cancel)
}

// configure libhoney
func libhoneyConfig() libhoney.ClientConfig {
	if apiKey == "" || dataset == "" {
		log.Warnln("LIBHONEY_API_KEY or LIBHONEY_DATASET not set, disabling libhoney")
		return libhoney.ClientConfig{}
	}

	return libhoney.ClientConfig{
		APIKey:       apiKey,
		Dataset:      dataset,
		APIHost:      apiHost,
		Transmission: newTransmission(),
	}
}

func newTransmission() *transmission.Honeycomb {
	batchSendTimeout := envOrElseDuration("HONEYCOMB_BATCH_SEND_TIMEOUT", defaultBatchSendTimeout)

	userAgent := fmt.Sprintf("honeycomb-lambda-extension-%s/%s", runtime.GOARCH, version)

	return &transmission.Honeycomb{
		MaxBatchSize:          libhoney.DefaultMaxBatchSize,
		BatchTimeout:          libhoney.DefaultBatchTimeout,
		MaxConcurrentBatches:  libhoney.DefaultMaxConcurrentBatches,
		PendingWorkCapacity:   libhoney.DefaultPendingWorkCapacity,
		UserAgentAddition:     userAgent,
		EnableMsgpackEncoding: true,
		BatchSendTimeout:      batchSendTimeout,
	}
}

// (helper) Retrieve an environment variable value by the given key,
// return an integer based on that value.
//
// If env var cannot be found by the key or value fails to cast to an int,
// return the given fallback integer.
func envOrElseInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		v, err := strconv.Atoi(value)
		if err != nil {
			log.Warnf("%s was set to '%s', but failed to parse to an integer. Falling back to default of %d.", key, value, fallback)
			return fallback
		}
		return v
	}
	return fallback
}

// (helper) Retrieve an environment variable value by the given key,
// return a boolean based on that value.
//
// If env var cannot be found by the key or value fails to cast to a bool,
// return the given fallback boolean.
func envOrElseBool(key string, fallback bool) bool {
	if value, ok := os.LookupEnv(key); ok {
		v, err := strconv.ParseBool(value)
		if err != nil {
			log.Warnf("%s was set to '%s', but failed to parse to true or false. Falling back to default of %t.", key, value, fallback)
			return fallback
		}
		return v
	}
	return fallback
}

// (helper) Retrieve an environment variable value by the given key,
// return the result of parsing the value as a duration.
//
// If value is an integer instead of a duration,
// return a duration assuming seconds as the unit.
//
// If env var cannot be found by the key,
// or the value fails to parse as a duration or integer,
// return the given fallback duration.
func envOrElseDuration(key string, fallback time.Duration) time.Duration {
	value, ok := os.LookupEnv(key)
	if ok {
		dur, err := time.ParseDuration(value)
		if err == nil {
			return dur
		}

		v, err := strconv.Atoi(value)
		if err == nil {
			dur_s := time.Duration(v) * time.Second
			log.Warnf("%s was set to %d (an integer, not a duration). Assuming 'seconds' as unit, resulting in %s.", key, v, dur_s)
			return dur_s
		}
		log.Warnf("%s was set to '%s', but failed to parse to a duration. Falling back to default of %s.", key, value, fallback)
	}
	return fallback
}

func readResponses(responses chan transmission.Response) {
	for r := range responses {
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
