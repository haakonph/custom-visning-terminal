// Package polling enforces server rate-limit cooldowns across polling requests.
package polling

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Transport is scoped to one authenticated client, never shared across accounts.
// It does not retry requests (in particular, it never retries mutations).
type Transport struct {
	Base  http.RoundTripper
	mu    sync.Mutex
	until time.Time
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	until := t.until
	t.mu.Unlock()
	if time.Now().Before(until) {
		h := make(http.Header)
		h.Set("Retry-After", strconv.Itoa(max(1, int(time.Until(until).Seconds())+1)))
		return &http.Response{StatusCode: 429, Status: "429 Too Many Requests", Header: h, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
	}
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	response, err := base.RoundTrip(req)
	if response != nil {
		h := response.Header
		limited := response.StatusCode == 429 || (response.StatusCode == 403 && (h.Get("Retry-After") != "" || h.Get("X-RateLimit-Remaining") == "0"))
		if limited {
			now := time.Now()
			deadline := now.Add(time.Minute)
			if raw := h.Get("Retry-After"); raw != "" {
				if n, e := strconv.ParseInt(raw, 10, 64); e == nil && n >= 0 {
					deadline = now.Add(time.Duration(min(n, 86400)) * time.Second)
				} else if at, e := http.ParseTime(raw); e == nil {
					deadline = at
				}
			} else if raw := h.Get("X-RateLimit-Reset"); raw != "" {
				if n, e := strconv.ParseInt(raw, 10, 64); e == nil {
					deadline = time.Unix(n, 0)
				}
			}
			t.mu.Lock()
			if deadline.After(t.until) {
				t.until = deadline
			}
			t.mu.Unlock()
		}
	}
	return response, err
}
