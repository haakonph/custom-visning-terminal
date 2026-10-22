package accesslog

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func readEntries(t *testing.T, path string) []entry {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var entries []entry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatal("invalid log line", err)
		}
		entries = append(entries, e)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return entries
}

func TestLogRequestsAndAppendAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "http.log")
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"POST", "/v1/logs", 200},
		{"GET", "/unknown%20endpoint", 404},
		{"CUSTOM", "/another-unknown", 403},
	} {
		l, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		h := l.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if tc.status != 200 {
				w.WriteHeader(tc.status)
			}
			w.Write([]byte("private-response"))
			w.WriteHeader(500) // First final status must win.
		}))
		req := httptest.NewRequest(tc.method, tc.path+"?token=private-token", strings.NewReader("private-body"))
		req.RemoteAddr = "127.0.0.1:54321"
		req.Header.Set("Authorization", "Bearer private-header")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != tc.status || w.Body.String() != "private-response" {
			t.Fatalf("logging changed response: %d %s", w.Code, w.Body.String())
		}
		if tc.status == 200 && w.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
			t.Fatal("logging lost automatic content type")
		}
		if err := l.Close(); err != nil {
			t.Fatal(err)
		}
		entries := readEntries(t, path)
		e := entries[len(entries)-1]
		if e.Method != tc.method || e.Path != tc.path || e.Status != tc.status || e.RemoteAddr != req.RemoteAddr || e.Time.IsZero() || e.DurationMS < 0 || e.Bytes != int64(w.Body.Len()) {
			t.Fatalf("incorrect log: %+v", e)
		}
	}
	if got := len(readEntries(t, path)); got != 3 {
		t.Fatal("restart truncated log", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private-token", "private-header", "private-body", "private-response"} {
		if strings.Contains(string(data), secret) {
			t.Fatal("log contains query, header or body", secret)
		}
	}
	for p, want := range map[string]os.FileMode{path: 0600, filepath.Dir(path): 0700} {
		info, err := os.Stat(p)
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("permissions on %s: %v, %v", p, info, err)
		}
	}
}

func TestConcurrentRequestsAndRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "http.log")
	l, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	h := l.Handler(http.NotFoundHandler())
	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() { h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/missing", nil)) })
	}
	wg.Wait()
	if err := l.writer.Rotate(); err != nil {
		t.Fatal(err)
	}
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/after-rotation", nil))
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	archives, err := filepath.Glob(filepath.Join(filepath.Dir(path), "http-*.log"))
	if err != nil || len(archives) != 1 {
		t.Fatalf("rotated logs: %v, %v", archives, err)
	}
	if got := len(readEntries(t, archives[0])); got != 100 {
		t.Fatal("lost concurrent requests", got)
	}
	entries := readEntries(t, path)
	if len(entries) != 1 || entries[0].Path != "/after-rotation" {
		t.Fatal("rotation lost new request", entries)
	}
	for _, p := range []string{path, archives[0]} {
		info, err := os.Stat(p)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("rotation changed permissions: %v, %v", info, err)
		}
	}
}

func TestWriteFailureDoesNotChangeResponseAndIsReportedOnClose(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	path := filepath.Join(dir, "http.log")
	l, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("directory no longer available"), 0600); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	l.Handler(http.NotFoundHandler()).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 404 {
		t.Fatal("log failure changed response", w.Code)
	}
	if err := l.Close(); err == nil {
		t.Fatal("write failure was not reported")
	}
}

func TestPanicIsLoggedAndPropagates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "http.log")
	l, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	func() {
		defer func() {
			if recover() != "handler panic" {
				t.Fatal("logging swallowed handler panic")
			}
		}()
		l.Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("handler panic") })).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/panic", nil))
	}()
	entries := readEntries(t, path)
	if len(entries) != 1 || entries[0].Status != 0 || !entries[0].Aborted {
		t.Fatal("panicking request was not logged", entries)
	}
}
