// Package conversations sends Markdown messages through incoming webhooks.
package conversations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	endpoint string
	http     *http.Client
}

func New(endpoint string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return nil, errors.New("MATTERMOST_WEBHOOK_URL must be an HTTPS webhook URL")
	}
	return &Client{endpoint, &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) Send(ctx context.Context, text string) error {
	if strings.TrimSpace(text) == "" {
		return errors.New("message is empty")
	}
	body, err := json.Marshal(struct {
		Text string `json:"text"`
	}{text})
	if err != nil {
		return errors.New("could not encode Mattermost message")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return errors.New("could not create Mattermost request")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return errors.New("Mattermost request failed; delivery may be uncertain, check the channel before retrying")
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Mattermost returned HTTP %d; check webhook and channel before retrying", resp.StatusCode)
	}
	return nil
}
