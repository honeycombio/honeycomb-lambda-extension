package logsapi

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	libhoney "github.com/honeycombio/libhoney-go"
	log "github.com/sirupsen/logrus"
)

// LogMessage is a message sent from the Logs API
type LogMessage struct {
	Type   string      `json:"type"`
	Time   string      `json:"time"`
	Record interface{} `json:"record"`
}

// handler receives batches of log messages from the Lambda Logs API. Each
// LogMessage is sent to Honeycomb as a separate event.
func handler(libhoneyClient *libhoney.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Warn("Error", err)
			return
		}
		defer r.Body.Close()
		var logs []LogMessage
		err = json.Unmarshal(body, &logs)
		if err != nil {
			log.Warn("Could not unmarshal payload", err)
			return
		}
		for _, msg := range logs {
			event := libhoneyClient.NewEvent()
			event.AddField("type", msg.Type)
			event.AddField("time", msg.Time)

			switch v := msg.Record.(type) {
			case string:
				// attempt to parse as json
				var record interface{}
				err = json.Unmarshal([]byte(v), &record)
				if err != nil {
					event.AddField("record", msg.Record)
				} else {
					event.Add(record)
				}
			default:
				event.Add(msg.Record)
			}
			event.Send()
		}
	}
}

// StartHTTPServer starts a logs API server on the specified port. The server will receive
// log messages from the Lambda Logs API and send them to Honeycomb as events.
func StartHTTPServer(port int, client *libhoney.Client) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handler(client))
	server := &http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%d", port),
		Handler: mux,
	}
	log.Info("Logs server listening on port ", port)
	log.Fatal(server.ListenAndServe())
}
