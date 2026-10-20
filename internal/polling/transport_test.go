package polling

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestServerCooldownPreventsRepeatedRequests(t *testing.T) {
	for _, h := range []http.Header{{"Retry-After": {"600"}}, {"Retry-After": {time.Now().Add(10 * time.Minute).UTC().Format(http.TimeFormat)}}, {"X-Ratelimit-Remaining": {"0"}, "X-Ratelimit-Reset": {"9999999999"}}} {
		calls := 0
		transport := &Transport{Base: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 429, Header: h, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		})}
		req, _ := http.NewRequest("GET", "https://example.com", nil)
		first, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		first.Body.Close()
		second, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		second.Body.Close()
		if calls != 1 || second.StatusCode != 429 || second.Header.Get("Retry-After") == "" {
			t.Fatal("server cooldown ignored")
		}
		transport.until = time.Now().Add(-time.Second)
		third, _ := transport.RoundTrip(req)
		third.Body.Close()
		if calls != 2 {
			t.Fatal("cooldown never expired")
		}
	}
}
