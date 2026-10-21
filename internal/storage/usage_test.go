package storage

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestUsageAtomicIncrementsAndRollback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "usage.sqlite")
	a, err := OpenUsage(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := OpenUsage(path)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	k := UsageKey{Day: "2026-10-21", Category: "command", Name: "search", Source: "github"}
	var wg sync.WaitGroup
	for _, s := range []*UsageStore{a, b} {
		wg.Go(func() {
			for range 10 {
				if err := s.Add(map[UsageKey]int64{k: 1}); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
	invalid := UsageKey{Day: "2026-10-21", Category: "invalid"}
	if err := a.Add(map[UsageKey]int64{k: 100, invalid: -1}); err == nil {
		t.Fatal("accepted negative count")
	}
	counts, err := b.Counts()
	if err != nil || len(counts) != 1 || counts[0].Count != 20 {
		t.Fatalf("increments lost or failed batch partially committed: %+v, %v", counts, err)
	}
	for _, p := range []string{filepath.Dir(path), path} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0077 != 0 {
			t.Fatalf("metrics readable by other users: %s", info.Mode())
		}
	}
}
