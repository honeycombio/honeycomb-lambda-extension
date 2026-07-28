// Command capture-extension is a throwaway Lambda extension whose only job is to
// record what the Telemetry API actually delivers, so the handler's tests can be
// driven by real payloads rather than hand-written approximations of them.
//
// Every request body the Telemetry API posts is echoed to stdout as
// CAPTURE:<base64>, which the capture script then pulls out of CloudWatch.
//
// It subscribes to function and platform telemetry but deliberately not to
// extension telemetry: its own stdout is extension telemetry, and subscribing to
// that would feed every capture back to itself forever.
//
// Stdlib only, so it builds without touching the extension's own module.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	receiverPort  = 3001
	schemaVersion = "2025-01-29"
)

func main() {
	runtimeAPI := os.Getenv("AWS_LAMBDA_RUNTIME_API")
	if runtimeAPI == "" {
		log.Fatal("AWS_LAMBDA_RUNTIME_API is not set")
	}

	go serveReceiver()

	extensionID, err := register(runtimeAPI)
	if err != nil {
		log.Fatalf("register: %v", err)
	}
	if err := subscribe(runtimeAPI, extensionID); err != nil {
		log.Fatalf("subscribe: %v", err)
	}
	fmt.Println("CAPTURE-READY")

	// An invocation isn't over when the runtime responds: Lambda keeps the
	// environment thawed until every registered extension asks for its next
	// event, and only then freezes. That window is what a real extension uses to
	// flush, and it's the only chance to receive the telemetry buffered during
	// the invoke. Calling next immediately would hand back permission to freeze
	// before any of it arrived.
	for {
		if err := nextEvent(runtimeAPI, extensionID); err != nil {
			log.Printf("next event: %v", err)
			return
		}
		waitForQuietTelemetry()
	}
}

// lastDelivery is when the receiver last saw a telemetry body.
var lastDelivery struct {
	sync.Mutex
	at time.Time
}

func noteDelivery() {
	lastDelivery.Lock()
	defer lastDelivery.Unlock()
	lastDelivery.at = time.Now()
}

func sinceDelivery() time.Duration {
	lastDelivery.Lock()
	defer lastDelivery.Unlock()
	if lastDelivery.at.IsZero() {
		return 0
	}
	return time.Since(lastDelivery.at)
}

// waitForQuietTelemetry holds the invocation open until telemetry stops arriving,
// so a capture doesn't end mid-delivery. Bounded, because holding the invocation
// open is billed time and Lambda will eventually lose patience.
func waitForQuietTelemetry() {
	const (
		quiet    = 400 * time.Millisecond
		deadline = 3 * time.Second
	)
	started := time.Now()
	for time.Since(started) < deadline {
		time.Sleep(50 * time.Millisecond)
		if sinceDelivery() > quiet {
			return
		}
	}
}

func serveReceiver() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("reading telemetry body: %v", err)
			return
		}
		defer r.Body.Close()
		noteDelivery()
		fmt.Printf("CAPTURE:%s\n", base64.StdEncoding.EncodeToString(body))
		w.WriteHeader(http.StatusOK)
	})
	log.Fatal(http.ListenAndServe(fmt.Sprintf("0.0.0.0:%d", receiverPort), mux))
}

func register(runtimeAPI string) (string, error) {
	body, err := json.Marshal(map[string][]string{"events": {"INVOKE", "SHUTDOWN"}})
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("http://%s/2020-01-01/extension/register", runtimeAPI)
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Lambda-Extension-Name", filepath.Base(os.Args[0]))

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(response.Body)
		return "", fmt.Errorf("status %d: %s", response.StatusCode, detail)
	}
	return response.Header.Get("Lambda-Extension-Identifier"), nil
}

func subscribe(runtimeAPI, extensionID string) error {
	body, err := json.Marshal(map[string]interface{}{
		"schemaVersion": schemaVersion,
		"types":         []string{"function", "platform"},
		"buffering":     map[string]int{"timeoutMs": 100, "maxBytes": 262144, "maxItems": 1000},
		"destination": map[string]string{
			"protocol": "HTTP",
			"URI":      fmt.Sprintf("http://sandbox:%d", receiverPort),
		},
	})
	if err != nil {
		return err
	}
	url := fmt.Sprintf("http://%s/2022-07-01/telemetry", runtimeAPI)
	request, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Lambda-Extension-Identifier", extensionID)
	request.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(response.Body)
		return fmt.Errorf("status %d: %s", response.StatusCode, detail)
	}
	return nil
}

func nextEvent(runtimeAPI, extensionID string) error {
	url := fmt.Sprintf("http://%s/2020-01-01/extension/event/next", runtimeAPI)
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Lambda-Extension-Identifier", extensionID)

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if _, err := io.ReadAll(response.Body); err != nil {
		return err
	}
	return nil
}
