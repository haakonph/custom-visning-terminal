// Package accesslog writes local, rotating HTTP access logs.
package accesslog

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

func DefaultPath() string {
	if path := os.Getenv("CUSTOM_VISNING_HTTP_LOG"); path != "" {
		return path
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "custom-visning", "http.log")
}

type Logger struct {
	mu     sync.Mutex
	writer *lumberjack.Logger
	err    error
	closed bool
}

func Open(path string) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	// Create the file now to report startup errors and give rotated files the
	// same private permissions (lumberjack preserves the current file's mode).
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	return &Logger{writer: &lumberjack.Logger{Filename: path, MaxSize: 10, MaxBackups: 3, MaxAge: 7}}, nil
}

type entry struct {
	Time       time.Time `json:"time"`
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	RemoteAddr string    `json:"remote_addr"`
	Status     int       `json:"status"`
	DurationMS float64   `json:"duration_ms"`
	Bytes      int64     `json:"bytes"`
	Aborted    bool      `json:"aborted,omitempty"`
}

// Handler logs every handled request, including requests rejected before routing.
// Query parameters, headers and bodies are deliberately excluded.
func (l *Logger) Handler(next http.Handler) http.Handler {
	if l == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		response := &responseWriter{ResponseWriter: w}
		completed := false
		defer func() {
			status := response.status
			if status == 0 && completed {
				status = http.StatusOK
			}
			l.record(entry{Time: started.UTC(), Method: r.Method, Path: r.URL.EscapedPath(), RemoteAddr: r.RemoteAddr, Status: status, DurationMS: float64(time.Since(started)) / float64(time.Millisecond), Bytes: response.bytes, Aborted: !completed})
		}()
		next.ServeHTTP(response, r)
		completed = true
	})
}

func (l *Logger) record(e entry) {
	data, err := json.Marshal(e)
	if err != nil {
		return // All fields have JSON-safe types.
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}
	_, err = l.writer.Write(append(data, '\n'))
	if l.err == nil {
		l.err = err
	}
}

// Close returns any write error, so a full disk cannot silently lose access logs.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.closed {
		l.closed = true
		if err := l.writer.Close(); l.err == nil {
			l.err = err
		}
	}
	return l.err
}

type responseWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (w *responseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *responseWriter) WriteHeader(status int) {
	if w.status == 0 && (status >= 200 || status == http.StatusSwitchingProtocols) {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(body)
	w.bytes += int64(n)
	return n, err
}
