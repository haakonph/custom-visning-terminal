package usage

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestConcurrentRecordsSurviveFlushAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.sqlite")
	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 100 {
				r.Record("command", "details", "github", "")
			}
		})
	}
	wg.Go(func() {
		for range 10 {
			if err := r.flush(); err != nil {
				t.Error(err)
			}
		}
	})
	wg.Wait()
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	r.Record("command", "ignored.after.close", "github", "")
	if err := r.Close(); err != nil {
		t.Fatal("repeated close", err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	counts, err := reopened.Snapshot(0)
	if err != nil || len(counts) != 1 || counts[0].Count != 800 {
		t.Fatalf("lost or duplicated counters: %+v, %v", counts, err)
	}
}

func TestDailyPeriodsIncludeTodayAndAggregate(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.Local)
	for _, age := range []int{0, 6, 7, 29, 30} {
		r.now = func() time.Time { return now.AddDate(0, 0, -age) }
		r.Record("command", "search", "jira", "")
	}
	r.now = func() time.Time { return now }
	for days, want := range map[int]int64{7: 2, 30: 4, 0: 5} {
		counts, err := r.Snapshot(days)
		if err != nil || len(counts) != 1 || counts[0].Count != want {
			t.Fatalf("period %d: %+v, %v", days, counts, err)
		}
	}
}
