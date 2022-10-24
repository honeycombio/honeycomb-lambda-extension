package extension

import (
	"os"
	"strconv"
	"time"
)

const (
	// default buffering options for logs api
	defaultTimeoutMS = 1000
	defaultMaxBytes  = 262144
	defaultMaxItems  = 1000

	// Waiting too long to send a batch of events can be
	// expensive in Lambda. It's reasonable to expect a
	// batch send to complete in this amount of time.
	defaultBatchSendTimeout = time.Second * 15

	// It's very generous to expect an HTTP connection to
	// to be established in this time.
	defaultConnectTimeout = time.Second * 3
)

type Config struct {
	APIKey                         string // Honeycomb API key
	Dataset                        string // target dataset at Honeycomb to receive events
	APIHost                        string // Honeycomb API URL to which to send events
	Debug                          bool   // Enable debug log output from the extension
	RuntimeAPI                     string // Set by AWS in extension environment. Expected to be hostname:port.
	LogsReceiverPort               int
	LogsAPITimeoutMS               int
	LogsAPIMaxBytes                int
	LogsAPIMaxItems                int
	LogsAPIDisablePlatformMessages bool

	// The start-to-finish timeout to send a batch of events to Honeycomb.
	BatchSendTimeout time.Duration

	// The timeout to establish an HTTP connection to Honeycomb API. This value ends
	// up being used as the Dial timeout for the underlying libhoney-go HTTP client. This setting
	// is critical to help reduce impact caused by connectivity issues as it allows us to
	// fail fast and not have to wait for the much longer HTTP client timeout to occur.
	ConnectTimeout time.Duration
}

// Returns a new Honeycomb extension config with values populated
// from environment variables.
func NewConfigFromEnvironment() Config {
	return Config{
		APIKey:                         os.Getenv("LIBHONEY_API_KEY"),
		Dataset:                        os.Getenv("LIBHONEY_DATASET"),
		APIHost:                        os.Getenv("LIBHONEY_API_HOST"),
		Debug:                          envOrElseBool("HONEYCOMB_DEBUG", false),
		RuntimeAPI:                     os.Getenv("AWS_LAMBDA_RUNTIME_API"),
		LogsReceiverPort:               3000, // a constant for now
		LogsAPITimeoutMS:               envOrElseInt("LOGS_API_TIMEOUT_MS", defaultTimeoutMS),
		LogsAPIMaxBytes:                envOrElseInt("LOGS_API_MAX_BYTES", defaultMaxBytes),
		LogsAPIMaxItems:                envOrElseInt("LOGS_API_MAX_ITEMS", defaultMaxItems),
		LogsAPIDisablePlatformMessages: envOrElseBool("LOGS_API_DISABLE_PLATFORM_MSGS", false),
		BatchSendTimeout:               envOrElseDuration("HONEYCOMB_BATCH_SEND_TIMEOUT", defaultBatchSendTimeout),
		ConnectTimeout:                 envOrElseDuration("HONEYCOMB_CONNECT_TIMEOUT", defaultConnectTimeout),
	}
}

// envOrElseInt retrieves an environment variable value by the given key,
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

// envOrElseBool retrieves an environment variable value by the given key,
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

// envOrElseDuration retrieves an environment variable value by the given key,
// return the result of parsing the value as a duration.
//
// If value is an integer instead of a duration,
// return a duration assuming seconds as the unit.
//
// If env var cannot be found by the key,
// or the value fails to parse as a duration or integer,
// or the result is a duration of 0,
// return the given fallback duration.
func envOrElseDuration(key string, fallback time.Duration) time.Duration {
	value, ok := os.LookupEnv(key)
	if ok {
		dur, err := time.ParseDuration(value)
		if err == nil {
			if dur == 0 {
				log.Warnf("%s was set to '%s', which is an unusable duration for the extension. Falling back to default of %s.", key, value, fallback)
				return fallback
			} else {
				return dur
			}
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
