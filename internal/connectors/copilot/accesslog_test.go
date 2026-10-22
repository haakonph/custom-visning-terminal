package copilot

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haakonph/custom-visning-terminal/internal/accesslog"
)

func TestReceiverLogsEveryRequestWithoutMetrics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "http.log")
	l, err := accesslog.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	o := testOptions(t)
	o.AccessLog = l
	s := service(t, o) // No usage recorder: access logging works independently.
	defer s.Close()
	h := s.handler()
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"/v1/logs", 200},
		{"/unknown-one", 404},
		{"/unknown-two", 404},
	} {
		req := httptest.NewRequest("POST", "http://localhost"+tc.path+"?token=secret", strings.NewReader(`{"resourceLogs":[]}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("%s: %d", tc.path, w.Code)
		}
	}
	// Origin checks run before routing, and must still produce a log entry.
	req := httptest.NewRequest("GET", "http://localhost/rejected", nil)
	req.Header.Set("Origin", "https://elsewhere.example")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 4 {
		t.Fatalf("missing or duplicate request logs: %s", data)
	}
	for i, want := range []struct {
		path   string
		status int
	}{{"/v1/logs", 200}, {"/unknown-one", 404}, {"/unknown-two", 404}, {"/rejected", 403}} {
		var e struct {
			Path   string `json:"path"`
			Status int    `json:"status"`
		}
		if err := json.Unmarshal([]byte(lines[i]), &e); err != nil || e.Path != want.path || e.Status != want.status {
			t.Fatalf("incorrect request log: %s, %v", lines[i], err)
		}
	}
}
