package usage

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
)

func TestHTTPHandlerCountsFinalStatusWithoutChangingResponse(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for _, tc := range []struct {
		name   string
		handle http.HandlerFunc
		status int
		body   string
	}{
		{"implicit", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) }, 200, "ok"},
		{"empty", func(http.ResponseWriter, *http.Request) {}, 200, ""},
		{"error", func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "bad", 400) }, 400, "bad\n"},
		{"first final wins", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503); w.WriteHeader(200) }, 503, ""},
		{"write then header", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")); w.WriteHeader(503) }, 200, "ok"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.HTTPHandler(tc.handle, "/known").ServeHTTP(w, httptest.NewRequest("POST", "/known?token=secret", nil))
			if w.Code != tc.status || w.Body.String() != tc.body {
				t.Fatalf("response changed: %d %q", w.Code, w.Body.String())
			}
			if tc.body != "" && w.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
				t.Fatal("lost automatic content type", w.Header())
			}
		})
	}
	counts, err := r.Snapshot(0)
	if err != nil || len(counts) != 3 {
		t.Fatalf("counts: %+v, %v", counts, err)
	}
	want := map[string]int64{"200": 3, "400": 1, "503": 1}
	for _, c := range counts {
		if c.Category != "http.incoming" || c.Name != "POST" || c.Source != "/known" || c.Count != want[c.Target] {
			t.Fatalf("unexpected request labels/count: %+v", c)
		}
	}
}

func TestHTTPHandlerBoundsLabelsAndPersistsConcurrentRequests(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.sqlite")
	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	h := r.HTTPHandler(http.NotFoundHandler(), "/known")
	var wg sync.WaitGroup
	for range 40 {
		wg.Go(func() {
			req := httptest.NewRequest("PRIVATE-METHOD", "http://localhost/private-path?token=secret", nil)
			req.Header.Set("Authorization", "Bearer secret")
			h.ServeHTTP(httptest.NewRecorder(), req)
		})
	}
	wg.Wait()
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	counts, err := reopened.Snapshot(0)
	if err != nil || len(counts) != 1 {
		t.Fatalf("counts: %+v, %v", counts, err)
	}
	c := counts[0]
	if c.Category != "http.incoming" || c.Name != "OTHER" || c.Source != "unknown" || c.Target != "404" || c.Count != 40 {
		t.Fatalf("private labels or missing requests: %+v", c)
	}
}

func TestHTTPHandlerWithoutRecorder(t *testing.T) {
	var r *Recorder
	w := httptest.NewRecorder()
	r.HTTPHandler(http.NotFoundHandler()).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 404 {
		t.Fatal("disabled metrics changed response", w.Code)
	}
}
