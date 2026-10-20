// Package github implements GitHub REST discovery and batched GraphQL enrichment.
package github

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	gh "github.com/google/go-github/v93/github"

	"github.com/haakonph/custom-visning-terminal/internal/polling"
)

type Client struct {
	api     *gh.Client
	teamsMu sync.Mutex
	teamsAt time.Time
	teams   []string
}

func New(token string) (*Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 8
	transport.ResponseHeaderTimeout = 20 * time.Second
	transport.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	return NewWithHTTP(token, &http.Client{Transport: transport, Timeout: 30 * time.Second})
}

// NewWithHTTP permits offline httptest fixtures; only the official API is used by New.
func NewWithHTTP(token string, client *http.Client, options ...gh.ClientOptionsFunc) (*Client, error) {
	copyClient := *client
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	copyClient.Transport = retryTransport{base: &polling.Transport{Base: base}}
	opts := append([]gh.ClientOptionsFunc{gh.WithHTTPClient(&copyClient), gh.WithAuthToken(token)}, options...)
	api, err := gh.NewClient(opts...)
	if err != nil {
		return nil, errors.New("could not configure GitHub HTTP client")
	}
	return &Client{api: api}, nil
}
func (c *Client) User(ctx context.Context) (string, error) {
	user, _, err := c.api.Users.Get(ctx, "")
	if err != nil {
		return "", safeError(err)
	}
	if user.GetLogin() == "" {
		return "", errors.New("GitHub did not return an authenticated username")
	}
	return user.GetLogin(), nil
}

// No remote response bodies or underlying transport errors are surfaced, to avoid
// accidentally echoing credentials or private response content in the terminal.
func safeError(err error) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	var response *gh.ErrorResponse
	if errors.As(err, &response) && response.Response != nil {
		code := response.Response.StatusCode
		switch code {
		case 401:
			return errors.New("GitHub authentication rejected; check GITHUB_TOKEN or gh auth login")
		case 403:
			return errors.New("GitHub access denied or rate limited; check token permissions, read:org and SSO authorization")
		case 429:
			return errors.New("GitHub rate limit reached; try again later")
		}
		return fmt.Errorf("GitHub API returned HTTP %d", code)
	}
	var rate *gh.RateLimitError
	var abuse *gh.AbuseRateLimitError
	if errors.As(err, &rate) || errors.As(err, &abuse) {
		return errors.New("GitHub rate limit reached; try again after the reset")
	}
	return errors.New("GitHub request failed or timed out; check network access")
}

type retryTransport struct{ base http.RoundTripper }

func (t retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		current := req.Clone(req.Context())
		if attempt > 0 && req.Body != nil {
			if req.GetBody == nil {
				return nil, errors.New("request cannot be retried")
			}
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			current.Body = body
		}
		response, err := t.base.RoundTrip(current)
		if req.Context().Err() != nil {
			if response != nil {
				response.Body.Close()
			}
			return nil, req.Context().Err()
		}
		retry := err != nil
		delay := time.Duration(1<<attempt) * 250 * time.Millisecond
		if response != nil {
			retry = response.StatusCode == 429 || response.StatusCode >= 500 || (response.StatusCode == 403 && (response.Header.Get("Retry-After") != "" || response.Header.Get("X-RateLimit-Remaining") == "0"))
			if d, ok := retryDelay(response.Header, time.Now()); ok {
				delay = d
			}
		}
		// Long rate-limit waits are reported to the UI instead of tying up a refresh.
		if !retry || attempt >= 2 || delay > 5*time.Second {
			return response, err
		}
		if response != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
			response.Body.Close()
		}
		timer := time.NewTimer(delay)
		select {
		case <-req.Context().Done():
			timer.Stop()
			return nil, req.Context().Err()
		case <-timer.C:
		}
	}
}
func retryDelay(h http.Header, now time.Time) (time.Duration, bool) {
	if value := h.Get("Retry-After"); value != "" {
		if seconds, err := strconv.Atoi(value); err == nil {
			return max(0, time.Duration(seconds)*time.Second), true
		}
		if at, err := http.ParseTime(value); err == nil {
			return max(0, at.Sub(now)), true
		}
	}
	if h.Get("X-RateLimit-Remaining") == "0" {
		if reset, err := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			return max(0, time.Unix(reset, 0).Sub(now)), true
		}
	}
	return 0, false
}
