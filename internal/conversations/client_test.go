package conversations

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebhook(t *testing.T) {
	for _, status := range []int{200, 400, 403, 429, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" {
					t.Error("invalid request")
				}
				var payload map[string]string
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				if payload["text"] != "[PR](https://github.com/a/b/pull/1)\n**review**" {
					t.Error(payload)
				}
				w.WriteHeader(status)
				w.Write([]byte("secret-webhook-token"))
			}))
			defer server.Close()
			c, err := New(server.URL + "/hooks/secret-webhook-token")
			if err != nil {
				t.Fatal(err)
			}
			c.http.Transport = server.Client().Transport
			err = c.Send(context.Background(), "[PR](https://github.com/a/b/pull/1)\n**review**")
			if (err == nil) != (status == 200) {
				t.Fatalf("status %d: %v", status, err)
			}
			if err != nil && strings.Contains(err.Error(), "secret-webhook-token") {
				t.Fatal("leaked token")
			}
			if calls != 1 {
				t.Fatal("unexpected retry")
			}
		})
	}
}
func TestValidationAndCancellation(t *testing.T) {
	for _, endpoint := range []string{"", "http://example.com/hooks/x", "https://user:pass@example.com/x", "https://example.com/x#secret"} {
		if _, err := New(endpoint); err == nil {
			t.Errorf("accepted %q", endpoint)
		}
	}
	c, err := New("https://conversations.example/hooks/secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Send(context.Background(), " \n"); err == nil {
		t.Fatal("empty message accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Send(ctx, "hello"); err == nil || strings.Contains(err.Error(), "/hooks/secret") {
		t.Fatalf("cancellation: %v", err)
	}
}
func TestRedirectDoesNotForwardMessage(t *testing.T) {
	forwarded := false
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded = true }))
	defer target.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer server.Close()
	c, _ := New(server.URL)
	c.http.Transport = server.Client().Transport
	if err := c.Send(context.Background(), "private text"); err == nil {
		t.Fatal("redirect accepted")
	}
	if forwarded {
		t.Fatal("message forwarded")
	}
}
