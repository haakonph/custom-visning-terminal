// Package usage records local, daily counters without storing typed content.
package usage

import (
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/haakonph/custom-visning-terminal/internal/storage"
)

func DefaultPath() string {
	if path := os.Getenv("CUSTOM_VISNING_METRICS"); path != "" {
		return path
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "custom-visning", "usage.sqlite")
}

type Recorder struct {
	mu      sync.Mutex
	flushMu sync.Mutex
	store   *storage.UsageStore
	totals  map[storage.UsageKey]int64
	pending map[storage.UsageKey]int64
	now     func() time.Time
	err     error
	closed  bool
	stop    chan struct{}
	done    chan struct{}
	once    sync.Once
}

func Open(path string) (*Recorder, error) {
	s, err := storage.OpenUsage(path)
	if err != nil {
		return nil, err
	}
	counts, err := s.Counts()
	if err != nil {
		s.Close()
		return nil, err
	}
	r := &Recorder{store: s, totals: map[storage.UsageKey]int64{}, pending: map[storage.UsageKey]int64{}, now: time.Now, stop: make(chan struct{}), done: make(chan struct{})}
	for _, c := range counts {
		r.totals[c.UsageKey] = c.Count
	}
	go r.run()
	return r, nil
}

// Record only updates memory. Disk writes run off the terminal event loop.
func (r *Recorder) Record(category, name, source, target string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	k := storage.UsageKey{Day: r.now().Format("2006-01-02"), Category: category, Name: name, Source: source, Target: target}
	r.totals[k]++
	r.pending[k]++
}

// Snapshot includes unflushed counters. Other running instances appear on restart.
// A zero period selects all days; otherwise today is included in the period.
func (r *Recorder) Snapshot(days int) ([]storage.UsageCount, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	since := ""
	if days > 0 {
		since = r.now().AddDate(0, 0, 1-days).Format("2006-01-02")
	}
	aggregated := map[storage.UsageKey]int64{}
	for k, n := range r.totals {
		if k.Day < since {
			continue
		}
		k.Day = ""
		aggregated[k] += n
	}
	counts := make([]storage.UsageCount, 0, len(aggregated))
	for k, n := range aggregated {
		counts = append(counts, storage.UsageCount{UsageKey: k, Count: n})
	}
	sort.Slice(counts, func(i, j int) bool {
		a, b := counts[i], counts[j]
		if a.Category != b.Category {
			return a.Category < b.Category
		}
		if a.Count != b.Count {
			return a.Count > b.Count
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		return a.Target < b.Target
	})
	return counts, r.err
}

func (r *Recorder) flush() error {
	r.flushMu.Lock()
	defer r.flushMu.Unlock()
	r.mu.Lock()
	pending := r.pending
	r.pending = map[storage.UsageKey]int64{}
	r.mu.Unlock()
	err := r.store.Add(pending)
	r.mu.Lock()
	defer r.mu.Unlock()
	if err != nil {
		for k, n := range pending {
			r.pending[k] += n
		}
	}
	r.err = err
	return err
}

func (r *Recorder) run() {
	defer close(r.done)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			r.flush()
		case <-r.stop:
			return
		}
	}
}

// Close waits for the writer and flushes the final counters before closing SQLite.
func (r *Recorder) Close() error {
	r.once.Do(func() {
		r.mu.Lock()
		r.closed = true
		r.mu.Unlock()
		close(r.stop)
		<-r.done
		err := r.flush()
		closeErr := r.store.Close()
		if err == nil {
			err = closeErr
		}
		r.mu.Lock()
		r.err = err
		r.mu.Unlock()
	})
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}
