package cache

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/storage"
)

func TestRoundTripAndAccountIsolation(t *testing.T) {
	store := &Store{Dir: filepath.Join(t.TempDir(), "cache")}
	empty, err := store.Load("alice")
	if err != nil || empty.User != "alice" {
		t.Fatalf("%+v %v", empty, err)
	}
	snap := model.Snapshot{Version: 1, User: "alice"}
	snap.Datasets[model.Open] = model.Dataset{PRs: []model.PullRequest{{Number: 42, Title: "Fix", CI: model.Unknown}}, SyncedAt: time.Now().UTC().Truncate(time.Second)}
	if err := store.Save(snap); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load("alice")
	if err != nil || got.Datasets[model.Open].PRs[0].Number != 42 || !got.Datasets[model.Open].SyncedAt.Equal(snap.Datasets[model.Open].SyncedAt) {
		t.Fatalf("%+v %v", got, err)
	}
	other, err := store.Load("bob")
	if err != nil || len(other.Datasets[model.Open].PRs) != 0 {
		t.Fatal("account cache leaked")
	}
	info, err := os.Stat(storage.Path(store.path("alice")))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0077 != 0 {
		t.Fatalf("cache permissions: %v", info.Mode())
	}
}
func TestCorruptAndInvalidCache(t *testing.T) {
	store := &Store{Dir: t.TempDir()}
	for _, data := range []string{`{`, `{"version":2,"user":"alice"}`, `{"version":1,"user":"bob"}`, `{"version":1,"user":"alice"} {}`} {
		if err := os.WriteFile(store.path("alice"), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Load("alice"); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
}
func TestConcurrentAtomicWrite(t *testing.T) {
	store := &Store{Dir: t.TempDir()}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := store.Save(model.Snapshot{Version: 1, User: "alice"}); err != nil {
				t.Error(err)
			}
			if _, err := store.Load("alice"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	entries, _ := os.ReadDir(store.Dir)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".snapshot-") {
			t.Fatal("temporary file leaked")
		}
	}
}

func TestCredentialScopedCacheContainsNoToken(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("HOME", cacheRoot)
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	t.Setenv("LocalAppData", cacheRoot)
	const token = "secret-credential-for-cache-test"
	store, err := Default(token)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Save(model.Snapshot{Version: 1, User: "alice"}); err != nil {
		t.Fatal(err)
	}
	snap, err := store.Load("")
	if err != nil || snap.User != "alice" {
		t.Fatalf("offline identity: %+v %v", snap, err)
	}
	entries, err := os.ReadDir(store.Dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(store.Dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), token) || strings.Contains(entry.Name(), token) {
			t.Fatal("credential leaked to disk")
		}
	}
	other, err := Default("different-token")
	if err != nil {
		t.Fatal(err)
	}
	otherSnap, err := other.Load("")
	if err != nil || otherSnap.User != "" {
		t.Fatal("credential scopes mixed")
	}
}

func TestLegacyCacheMigration(t *testing.T) {
	store := &Store{Dir: t.TempDir()}
	path := store.path("alice")
	original := `{"version":1,"user":"alice"}`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	if snap, err := store.Load("alice"); err != nil || snap.User != "alice" {
		t.Fatal(snap, err)
	}
	if _, err := os.Stat(storage.Path(path)); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != original {
		t.Fatal("legacy cache changed")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if snap, err := store.Load("alice"); err != nil || snap.User != "alice" {
		t.Fatal(snap, err)
	}
}
