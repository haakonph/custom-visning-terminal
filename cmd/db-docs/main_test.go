package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateAndCheck(t *testing.T) {
	out := filepath.Join(t.TempDir(), "docs")
	if err := run(out, "", false, true); err == nil {
		t.Fatal("missing docs accepted")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("check wrote output")
	}
	if err := run(out, "", false, false); err != nil {
		t.Fatal(err)
	}
	if err := run(out, "", false, true); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"schema.md", "schema.sql"} {
		path := filepath.Join(out, name)
		original, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, []byte("stale"), 0644); err != nil {
			t.Fatal(err)
		}
		if err = run(out, "", false, true); err == nil {
			t.Fatalf("stale %s accepted", name)
		}
		current, _ := os.ReadFile(path)
		if string(current) != "stale" {
			t.Fatal("check rewrote stale docs")
		}
		if err = os.WriteFile(path, original, 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestExplicitEmptyBaseline(t *testing.T) {
	if err := run(t.TempDir(), "missing", false, false); err == nil {
		t.Fatal("missing migrations accepted")
	}
	if err := run(t.TempDir(), "missing", true, false); err == nil {
		t.Fatal("incompatible flags accepted")
	}
	out := t.TempDir()
	if err := run(out, "", true, false); err != nil {
		t.Fatal(err)
	}
	if err := run(out, "", true, true); err != nil {
		t.Fatal(err)
	}
}
