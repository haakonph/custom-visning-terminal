package copilot

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSharedAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected mutation: %s", r.Method)
		}
		switch r.URL.Path {
		case "/api/copilot/sessions":
			fmt.Fprint(w, `[{"id":"s1","name":"Fix login","status":"idle","inputTokens":42}]`)
		case "/api/copilot/sessions/s1":
			fmt.Fprint(w, `{"session":{"id":"s1"},"events":[{"type":"tool.complete","tool":"shell","result":"ok"}]}`)
		case "/api/copilot/diagnostics":
			fmt.Fprint(w, `{"receiverRunning":true,"port":4318,"showContent":false}`)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	c, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := c.Sessions(context.Background())
	if err != nil || len(sessions) != 1 || *sessions[0].InputTokens != 42 {
		t.Fatalf("sessions: %+v %v", sessions, err)
	}
	detail, err := c.Detail(context.Background(), sessions[0].ID)
	if err != nil || len(detail.Events) != 1 || detail.Events[0].Result != "ok" {
		t.Fatalf("detail: %+v %v", detail, err)
	}
	d, err := c.Diagnostics(context.Background())
	if err != nil || !d.ReceiverRunning || d.ShowContent {
		t.Fatalf("diagnostics: %+v %v", d, err)
	}
}
func TestLocalOriginsOnly(t *testing.T) {
	for _, address := range []string{"https://localhost:3001", "http://example.com", "http://127.0.0.1.evil.test", "http://user:password@localhost", "http://localhost/api", "http://localhost?token=secret"} {
		if _, err := New(address); err == nil {
			t.Errorf("accepted %s", address)
		}
	}
	for _, address := range []string{"", "http://localhost:3001", "http://127.0.0.1:3100", "http://[::1]:3001"} {
		if _, err := New(address); err != nil {
			t.Errorf("rejected %s: %v", address, err)
		}
	}
}
func TestFailuresDoNotExposeResponseBodyOrFollowRedirects(t *testing.T) {
	for _, body := range []string{"private session data", "{invalid"} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if body == "private session data" {
				w.WriteHeader(500)
			}
			fmt.Fprint(w, body)
		}))
		c, _ := New(s.URL)
		_, err := c.Sessions(context.Background())
		s.Close()
		if err == nil || strings.Contains(err.Error(), body) {
			t.Fatalf("unsafe error: %v", err)
		}
	}
	reached := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer redirect.Close()
	c, _ := New(redirect.URL)
	_, err := c.Sessions(context.Background())
	if err == nil || reached {
		t.Fatalf("redirect followed: %t, %v", reached, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Sessions(ctx); err == nil {
		t.Fatal("ignored cancellation")
	}
}
