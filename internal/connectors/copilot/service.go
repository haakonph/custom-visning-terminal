package copilot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"
)

type Options struct {
	SessionRoot, StorePath               string
	Port                                 int
	Enabled, CaptureContent, ShowContent bool
	JiraPattern                          string
}

func DefaultOptions() (Options, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Options{}, err
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return Options{}, err
	}
	return Options{SessionRoot: filepath.Join(home, ".copilot", "session-state"), StorePath: filepath.Join(dir, "custom-visning-terminal", "copilot.json"), Port: 4318, Enabled: true, CaptureContent: true, ShowContent: true, JiraPattern: `[A-Z][A-Z0-9_]*-[1-9][0-9]*`}, nil
}
func OptionsFromEnv() (Options, error) {
	o, err := DefaultOptions()
	if err != nil {
		return o, err
	}
	for key, target := range map[string]*string{"COPILOT_SESSION_ROOT": &o.SessionRoot, "COPILOT_STORE_PATH": &o.StorePath, "COPILOT_JIRA_PATTERN": &o.JiraPattern} {
		if v, ok := os.LookupEnv(key); ok {
			*target = v
		}
	}
	for key, target := range map[string]*bool{"COPILOT_COLLECTION_ENABLED": &o.Enabled, "COPILOT_CAPTURE_CONTENT": &o.CaptureContent, "COPILOT_SHOW_CONTENT": &o.ShowContent} {
		if v, ok := os.LookupEnv(key); ok {
			b, e := strconv.ParseBool(v)
			if e != nil {
				return o, fmt.Errorf("%s must be true or false", key)
			}
			*target = b
		}
	}
	if v, ok := os.LookupEnv("COPILOT_OTLP_PORT"); ok {
		p, e := strconv.Atoi(v)
		if e != nil || p < 1 || p > 65535 {
			return o, errors.New("COPILOT_OTLP_PORT must be 1–65535")
		}
		o.Port = p
	}
	return o, nil
}

// Service owns its importer, private store and optional loopback OTLP receiver.
type Service struct {
	mu          sync.Mutex
	scanMu      sync.Mutex
	options     Options
	pattern     *regexp.Regexp
	data        diskStore
	cursors     map[string]*cursor
	diagnostics Diagnostics
	receiver    *http.Server
	cancel      context.CancelFunc
	done        chan struct{}
	started     bool
	dirty       bool
}

func New(o Options) (*Service, error) {
	if !filepath.IsAbs(o.SessionRoot) || !filepath.IsAbs(o.StorePath) {
		return nil, errors.New("Copilot session root and store path must be absolute")
	}
	if o.Port < 0 || o.Port > 65535 {
		return nil, errors.New("invalid OTLP port")
	}
	re, err := regexp.Compile(o.JiraPattern)
	if err != nil {
		return nil, errors.New("invalid COPILOT_JIRA_PATTERN")
	}
	s := &Service{options: o, pattern: re, cursors: map[string]*cursor{}, data: diskStore{Version: 1, Sessions: map[string]*storedSession{}, Traces: map[string]string{}}, diagnostics: Diagnostics{SessionRoot: o.SessionRoot, StorePath: o.StorePath, Port: o.Port, CollectionEnabled: o.Enabled, CaptureContent: o.CaptureContent, ShowContent: o.ShowContent}}
	f, err := os.Open(o.StorePath)
	if err == nil {
		defer f.Close()
		// The store grows with imported history. Read the complete file so that
		// a store we successfully saved can also be opened on the next launch.
		dec := json.NewDecoder(f)
		err = dec.Decode(&s.data)
		if err != nil {
			return nil, storeReadError(o.StorePath, "invalid or incomplete JSON")
		}
		var extra any
		if dec.Decode(&extra) != io.EOF || s.data.Version != 1 || s.data.Sessions == nil || s.data.Traces == nil {
			return nil, storeReadError(o.StorePath, "unsupported or invalid store format")
		}
		for _, v := range s.data.Sessions {
			if v == nil || v.Events == nil || v.Meta == nil {
				return nil, storeReadError(o.StorePath, "invalid session format")
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, storeReadError(o.StorePath, "file could not be opened; check path and permissions")
	}
	return s, nil
}

func storeReadError(path, reason string) error {
	// Do not include decoder errors: they can quote private file contents.
	return fmt.Errorf("could not read Copilot store %q (%s); existing file has been preserved. Set COPILOT_STORE_PATH to a new absolute path to start with a separate store", path, reason)
}

func (s *Service) Start(parent context.Context) {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.done = make(chan struct{})
	s.mu.Unlock()
	if s.options.Enabled {
		listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(s.options.Port)))
		s.mu.Lock()
		if err != nil {
			s.addErrorLocked("OTLP-porten kunne ikke åpnes; lokal filimport fortsetter. Velg COPILOT_OTLP_PORT ved portkonflikt.")
		} else {
			s.diagnostics.Port = listener.Addr().(*net.TCPAddr).Port
			s.diagnostics.ReceiverRunning = true
			s.receiver = &http.Server{Handler: s.handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
			go func() {
				if e := s.receiver.Serve(listener); e != nil && !errors.Is(e, http.ErrServerClosed) {
					s.mu.Lock()
					s.diagnostics.ReceiverRunning = false
					s.addErrorLocked("OTLP-mottakeren stoppet")
					s.mu.Unlock()
				}
			}()
		}
		s.mu.Unlock()
	}
	go func() {
		defer close(s.done)
		if s.options.Enabled {
			s.Scan(ctx)
		}
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if s.options.Enabled {
					s.Scan(ctx)
				}
			}
		}
	}()
}
func (s *Service) Close() error {
	s.mu.Lock()
	cancel, done, server := s.cancel, s.done, s.receiver
	s.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
	if server != nil {
		ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		if err := server.Shutdown(ctx); err != nil {
			_ = server.Close()
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.diagnostics.ReceiverRunning = false
	return s.saveLocked()
}
func (s *Service) addErrorLocked(message string) {
	for _, v := range s.diagnostics.Errors {
		if v == message {
			return
		}
	}
	s.diagnostics.Errors = append(s.diagnostics.Errors, message)
	if len(s.diagnostics.Errors) > 50 {
		s.diagnostics.Errors = s.diagnostics.Errors[len(s.diagnostics.Errors)-50:]
	}
}
func (s *Service) Diagnostics(ctx context.Context) (Diagnostics, error) {
	if err := ctx.Err(); err != nil {
		return Diagnostics{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.diagnostics
	d.Errors = append([]string{}, d.Errors...)
	return d, nil
}
