package copilot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/haakonph/custom-visning-terminal/internal/storage"
)

func TestCloseCancelsWorkerBeforeWaitingForDataLock(t *testing.T) {
	o := testOptions(t)
	o.Enabled = false
	s := service(t, o)
	s.Start(context.Background())
	cancelled := make(chan struct{})
	cancel := s.cancel
	s.cancel = func() { close(cancelled); cancel() }

	// A scan can own the data lock when the user quits. Close must signal
	// cancellation without waiting for that lock to become available.
	s.mu.Lock()
	closed := make(chan error, 1)
	go func() { closed <- s.Close() }()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Error("Close waited for the data lock before cancelling the worker")
	}
	s.mu.Unlock()
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
}

func TestDeferredStartupPreservesUnreadableStore(t *testing.T) {
	o := testOptions(t)
	o.Enabled = false
	path := storage.Path(o.StorePath)
	write(t, path, "broken database containing private history")
	s, err := NewDeferred(o)
	if err != nil {
		t.Fatalf("construction accessed history: %v", err)
	}
	s.Start(context.Background())
	if _, err := s.Sessions(context.Background()); err == nil {
		t.Fatal("unreadable history was silently accepted")
	}
	if err := s.Close(); err == nil {
		t.Fatal("close must report the store error")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "broken database containing private history" {
		t.Fatalf("existing store changed: %v", err)
	}
}

func TestDeferredStartupConcurrentReadersAndCollection(t *testing.T) {
	o := testOptions(t)
	write(t, filepath.Join(o.SessionRoot, "s1", "events.jsonl"), eventJSON("1", "user.message", map[string]any{"content": "saved history"}))
	initial := service(t, o)
	initial.Scan(context.Background())
	if err := initial.Close(); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(o.SessionRoot, "s1", "events.jsonl"), eventJSON("1", "user.message", map[string]any{"content": "saved history"})+eventJSON("2", "assistant.message", map[string]any{"content": "new event"}))
	s, err := NewDeferred(o)
	if err != nil {
		t.Fatal(err)
	}
	s.Start(context.Background())
	var readers sync.WaitGroup
	for range 8 {
		readers.Go(func() {
			rows, err := s.Sessions(context.Background())
			if err != nil || len(rows) != 1 || rows[0].Name != "saved history" {
				t.Errorf("persisted history unavailable: %v %v", rows, err)
			}
			if _, err := s.Diagnostics(context.Background()); err != nil {
				t.Error(err)
			}
		})
	}
	readers.Wait()
	s.Scan(context.Background())
	detail, err := s.Detail(context.Background(), "s1")
	if err != nil || len(detail.Events) != 2 {
		t.Fatalf("history lost or duplicated during startup: %v %v", detail, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := service(t, o)
	defer reopened.Close()
	detail, err = reopened.Detail(context.Background(), "s1")
	if err != nil || len(detail.Events) != 2 {
		t.Fatalf("new history not persisted: %v %v", detail, err)
	}
}

func TestDeferredOTLPLoadsHistoryBeforeSaving(t *testing.T) {
	o := testOptions(t)
	write(t, filepath.Join(o.SessionRoot, "s1", "events.jsonl"), eventJSON("1", "user.message", map[string]any{"content": "saved history"}))
	initial := service(t, o)
	initial.Scan(context.Background())
	if err := initial.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := NewDeferred(o)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	body, err := json.Marshal(otlpFixture("logs"))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4318/v1/logs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	s.handler().ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("export failed: %d", response.Code)
	}
	reopened := service(t, o)
	defer reopened.Close()
	detail, err := reopened.Detail(context.Background(), "s1")
	if err != nil || len(detail.Events) != 2 {
		t.Fatalf("early export replaced history: %v %v", detail, err)
	}
}

// Compare the work before the first frame with both constructors, against the
// same persisted history. No upstream services or user history are required.
func BenchmarkStartup(b *testing.B) {
	dir := b.TempDir()
	o := Options{SessionRoot: filepath.Join(dir, "sessions"), StorePath: filepath.Join(dir, "copilot.sqlite"), JiraPattern: `[A-Z]+-[0-9]+`}
	s, err := New(o)
	if err != nil {
		b.Fatal(err)
	}
	for i := range 100 {
		id := fmt.Sprintf("session-%d", i)
		for j := range 100 {
			s.ingestLocked([]Event{{SessionID: id, Source: "local", Key: fmt.Sprint(j), Time: "2026-10-17T10:00:00Z", Role: "user", Content: "Investigate APP-42", Meta: map[string]any{}}})
		}
	}
	if err := s.Close(); err != nil {
		b.Fatal(err)
	}
	b.Run("synchronous-history", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := New(o); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("deferred-history", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := NewDeferred(o); err != nil {
				b.Fatal(err)
			}
		}
	})
}
