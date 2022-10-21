package main

import (
	"os"
	"strconv"
	"time"
)

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
