// Package copilot collects local Copilot CLI history and OTLP telemetry.
package copilot

import "github.com/haakonph/custom-visning-terminal/internal/model"

type Session = model.CopilotSession
type Event = model.CopilotEvent
type Detail struct {
	Session Session `json:"session"`
	Events  []Event `json:"events"`
}
type Diagnostics struct {
	SessionRoot       string   `json:"sessionRoot"`
	StorePath         string   `json:"storePath"`
	ReceiverRunning   bool     `json:"receiverRunning"`
	Port              int      `json:"port"`
	Discovered        int      `json:"discovered"`
	LastOtel          string   `json:"lastOtel"`
	ContentReceived   bool     `json:"contentReceived"`
	Errors            []string `json:"errors"`
	CollectionEnabled bool     `json:"collectionEnabled"`
	CaptureContent    bool     `json:"captureContent"`
	ShowContent       bool     `json:"showContent"`
}
