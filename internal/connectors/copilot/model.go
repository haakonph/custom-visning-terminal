// Package copilot collects local Copilot CLI history and OTLP telemetry.
package copilot

import "encoding/json"

type Session struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Client       string   `json:"client"`
	Model        string   `json:"model"`
	Agent        string   `json:"agent"`
	Cwd          string   `json:"cwd"`
	ProjectRoot  string   `json:"projectRoot"`
	GitRoot      string   `json:"gitRoot"`
	Branch       string   `json:"branch"`
	Remote       string   `json:"remote"`
	Repository   string   `json:"repository"`
	Jira         string   `json:"jira"`
	StartedAt    string   `json:"startedAt"`
	LastActivity string   `json:"lastActivity"`
	TraceIDs     []string `json:"traceIds"`
	Status       string   `json:"status"`
	StatusSource string   `json:"statusSource"`
	InputTokens  *int64   `json:"inputTokens"`
	OutputTokens *int64   `json:"outputTokens"`
	DurationMs   *float64 `json:"durationMs"`
}
type Event struct {
	SessionID    string          `json:"sessionId,omitempty"`
	TraceID      string          `json:"traceId,omitempty"`
	SpanID       string          `json:"spanId,omitempty"`
	EventID      string          `json:"eventId,omitempty"`
	StartTime    string          `json:"startTime,omitempty"`
	State        string          `json:"state,omitempty"`
	Meta         map[string]any  `json:"meta,omitempty"`
	MetricSeries string          `json:"metricSeries,omitempty"`
	Temporality  int             `json:"temporality,omitempty"`
	Key          string          `json:"key"`
	Source       string          `json:"source"`
	Time         string          `json:"time"`
	Type         string          `json:"type"`
	Content      string          `json:"content"`
	Role         string          `json:"role"`
	Tool         string          `json:"tool"`
	CallID       string          `json:"callId"`
	Args         string          `json:"args"`
	Result       string          `json:"result"`
	Error        string          `json:"error"`
	Warning      string          `json:"warning"`
	Agent        string          `json:"agent"`
	Parent       string          `json:"parent"`
	Model        string          `json:"model"`
	InputTokens  *int64          `json:"inputTokens"`
	OutputTokens *int64          `json:"outputTokens"`
	DurationMs   *float64        `json:"durationMs"`
	Raw          json.RawMessage `json:"raw"`
}
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
