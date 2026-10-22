package copilot

import (
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/haakonph/custom-visning-terminal/internal/storage"
	"github.com/haakonph/custom-visning-terminal/internal/usage"
)

func TestReceiverMetricsIncludeKnownUnknownAndRejectedRequests(t *testing.T) {
	r, err := usage.Open(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	o := testOptions(t)
	o.Usage = r
	s := service(t, o)
	defer s.Close()
	h := s.handler()
	want := map[storage.UsageKey]int64{}
	for _, tc := range []struct {
		method, path, body, origin, contentType string
		status                                  int
	}{
		{"POST", "/v1/traces", `{"resourceSpans":[]}`, "", "application/json", 200},
		{"POST", "/v1/logs?token=private-token", `{"resourceLogs":[]}`, "", "application/json", 200},
		{"POST", "/v1/metrics", `{"resourceMetrics":[]}`, "", "application/json", 200},
		{"GET", "/private-path?token=private-token", "", "", "", 404},
		{"GET", "/another-private-path", "", "", "", 404},
		{"GET", "/v1/logs", "", "", "", 405},
		{"POST", "/v1/logs", "{", "", "application/json", 400},
		{"POST", "/v1/logs", "{}", "", "text/plain", 415},
		{"POST", "/v1/logs", "{}", "https://private-origin", "application/json", 403},
		{"GET", "/private-path", "", "https://private-origin", "", 403},
	} {
		req := httptest.NewRequest(tc.method, "http://127.0.0.1:4318"+tc.path, strings.NewReader(tc.body))
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("Content-Type", tc.contentType)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("%s %s: got %d, want %d", tc.method, tc.path, w.Code, tc.status)
		}
		path := req.URL.Path
		if path != "/v1/traces" && path != "/v1/logs" && path != "/v1/metrics" {
			path = "unknown"
		}
		want[storage.UsageKey{Category: "http.incoming", Name: tc.method, Source: path, Target: strconv.Itoa(tc.status)}]++
	}
	counts, err := r.Snapshot(0)
	if err != nil || len(counts) != len(want) {
		t.Fatalf("counts: %+v, %v", counts, err)
	}
	for _, c := range counts {
		if c.Count != want[c.UsageKey] {
			t.Errorf("missing, duplicate or private request labels: %+v", c)
		}
	}
}
