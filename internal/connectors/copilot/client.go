// Package copilot reads the same local API as the custom-visning web UI.
package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

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
type Client struct {
	base string
	http *http.Client
}

func New(address string) (*Client, error) {
	if address == "" {
		address = "http://127.0.0.1:3001"
	}
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Hostname() != "127.0.0.1" && u.Hostname() != "::1" && u.Hostname() != "localhost") {
		return nil, fmt.Errorf("COPILOT_API_URL must be a local HTTP origin, e.g. http://127.0.0.1:3001")
	}
	return &Client{strings.TrimRight(address, "/"), &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) get(ctx context.Context, path string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/copilot/"+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("Copilot API unavailable; start custom-visning and check COPILOT_API_URL")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Copilot API returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024+1))
	if err != nil || len(body) > 16*1024*1024 {
		return fmt.Errorf("Copilot API response could not be read or is too large")
	}
	if json.Unmarshal(body, target) != nil {
		return fmt.Errorf("Copilot API returned invalid JSON")
	}
	return nil
}
func (c *Client) Sessions(ctx context.Context) ([]Session, error) {
	var v []Session
	err := c.get(ctx, "sessions", &v)
	return v, err
}
func (c *Client) Detail(ctx context.Context, id string) (Detail, error) {
	var v Detail
	err := c.get(ctx, "sessions/"+url.PathEscape(id), &v)
	return v, err
}
func (c *Client) Diagnostics(ctx context.Context) (Diagnostics, error) {
	var v Diagnostics
	err := c.get(ctx, "diagnostics", &v)
	return v, err
}
