//go:build rie

package rie

import (
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/vmihailenco/msgpack/v5"
)

// deliveredEvent is one event the extension sent, with the dataset it was
// addressed to. The dataset is the interesting part: only translated telemetry
// routes away from the configured default.
type deliveredEvent struct {
	Dataset string
	Data    map[string]interface{}
}

// sink stands in for Honeycomb's API. It decodes the batches the extension sends
// so tests can assert on events rather than on byte counts.
type sink struct {
	server *http.Server
	addr   string

	lock   sync.Mutex
	events []deliveredEvent
}

// startSink listens on all interfaces so the container can reach it, and returns
// the port to point the extension at.
func startSink(t *testing.T) *sink {
	t.Helper()

	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("listening for deliveries: %v", err)
	}

	s := &sink{addr: listener.Addr().String()}
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handle)
	s.server = &http.Server{Handler: mux}

	go s.server.Serve(listener)
	t.Cleanup(func() { s.server.Close() })
	return s
}

func (s *sink) port() string {
	_, port, _ := net.SplitHostPort(s.addr)
	return port
}

// handle decodes a batch. libhoney sends msgpack, zstd-compressed.
func (s *sink) handle(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var reader io.Reader = r.Body
	switch r.Header.Get("Content-Encoding") {
	case "zstd":
		decoder, err := zstd.NewReader(r.Body)
		if err == nil {
			defer decoder.Close()
			reader = decoder
		}
	}

	body, err := io.ReadAll(reader)
	if err == nil {
		var batch []map[string]interface{}
		if err := msgpack.Unmarshal(body, &batch); err == nil {
			dataset := strings.TrimPrefix(r.URL.Path, "/1/batch/")
			s.lock.Lock()
			for _, event := range batch {
				data, _ := event["data"].(map[string]interface{})
				s.events = append(s.events, deliveredEvent{Dataset: dataset, Data: data})
			}
			s.lock.Unlock()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`[{"status":202}]`))
}

func (s *sink) delivered() []deliveredEvent {
	s.lock.Lock()
	defer s.lock.Unlock()
	events := make([]deliveredEvent, len(s.events))
	copy(events, s.events)
	return events
}

// byName indexes delivered events by their name field, which is how a span is
// identified once translated.
func (s *sink) byName() map[string]deliveredEvent {
	byName := map[string]deliveredEvent{}
	for _, event := range s.delivered() {
		if name, ok := event.Data["name"].(string); ok {
			byName[name] = event
		}
	}
	return byName
}

// spansNamed counts events with a given name, so a payload that should produce
// two spans can be told apart from one that produced one.
func (s *sink) spansNamed(name string) int {
	count := 0
	for _, event := range s.delivered() {
		if event.Data["name"] == name {
			count++
		}
	}
	return count
}
