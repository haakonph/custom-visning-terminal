package storage

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestMigrationKeepsBackupAndDatabaseWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	original := `{"title":"saved","refs":["ABC-1","org/repo#2"]}`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := Load(path, &value); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(path, value); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(Path(path))
	if err != nil || string(data[:16]) != "SQLite format 3\x00" {
		t.Fatalf("not SQLite: %v", err)
	}
	backup, _ := os.ReadFile(path)
	if string(backup) != original {
		t.Fatal("legacy backup modified")
	}
	value["title"] = "updated"
	if err := Save(path, value); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(path, map[string]any{"title": "stale"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("broken stale backup"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Load(path, &value); err != nil || value["title"] != "updated" {
		t.Fatalf("database lost precedence: %v %v", value, err)
	}
	info, _ := os.Stat(Path(path))
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
}

func TestMissingCorruptAndFailedWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	var value map[string]any
	if err := Load(path, &value); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	for _, input := range []string{"broken", `{} {}`} {
		if err := os.WriteFile(path, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		if err := Load(path, &value); err == nil {
			t.Fatal("accepted corrupt legacy JSON")
		}
		if _, err := os.Stat(Path(path)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("created database before validation")
		}
	}
	if err := Save(path, map[string]string{"title": "keep"}); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, make(chan int)); err == nil {
		t.Fatal("accepted unencodable data")
	}
	if err := Load(path, &value); err != nil || value["title"] != "keep" {
		t.Fatal(value, err)
	}
	if err := os.WriteFile(Path(path), []byte("broken database"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Load(path, &value); err == nil {
		t.Fatal("fell back from corrupt SQLite")
	}
	data, _ := os.ReadFile(Path(path))
	if string(data) != "broken database" {
		t.Fatal("corrupt database modified")
	}
}

func TestConcurrentSavesAreAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.sqlite")
	if err := Save(path, map[string]int{"a": 0, "b": 0}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if err := Save(path, map[string]int{"a": n, "b": n}); err != nil {
				t.Error(err)
				return
			}
			var value map[string]int
			if err := Load(path, &value); err != nil {
				t.Error(err)
				return
			}
			if value["a"] != value["b"] {
				t.Error("partial write")
			}
		}(i)
	}
	wg.Wait()
}
