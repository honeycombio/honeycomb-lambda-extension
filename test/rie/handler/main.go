// Command handler is a minimal custom-runtime Lambda handler for the RIE tests.
// It writes one OTLP/JSON line per invocation so the logs show what the
// extension did or didn't do with it, then returns.
//
// Stdlib only, so it builds without touching the extension's own module.
package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

const otlpTraces = `{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"rie-func"}}]},"scopeSpans":[{"spans":[{"traceId":"5b8efff798038103d269b633813fc60c","spanId":"eee19b7ec3c1b174","name":"rie-handler","startTimeUnixNano":"1753000000000000000","endTimeUnixNano":"1753000000123000000"}]}]}]}`

func main() {
	runtimeAPI := os.Getenv("AWS_LAMBDA_RUNTIME_API")
	if runtimeAPI == "" {
		log.Fatal("AWS_LAMBDA_RUNTIME_API is not set")
	}

	for {
		requestID, err := next(runtimeAPI)
		if err != nil {
			log.Fatalf("next invocation: %v", err)
		}
		fmt.Println(otlpTraces)
		if err := respond(runtimeAPI, requestID); err != nil {
			log.Fatalf("responding: %v", err)
		}
	}
}

func next(runtimeAPI string) (string, error) {
	response, err := http.Get(fmt.Sprintf("http://%s/2018-06-01/runtime/invocation/next", runtimeAPI))
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if _, err := io.ReadAll(response.Body); err != nil {
		return "", err
	}
	return response.Header.Get("Lambda-Runtime-Aws-Request-Id"), nil
}

func respond(runtimeAPI, requestID string) error {
	url := fmt.Sprintf("http://%s/2018-06-01/runtime/invocation/%s/response", runtimeAPI, requestID)
	response, err := http.Post(url, "application/json", bytes.NewReader([]byte(`"ok"`)))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, err = io.ReadAll(response.Body)
	return err
}
