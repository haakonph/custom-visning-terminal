package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpdatePreservesSectionsAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := Update(path, map[string]any{"editors": map[string]string{"default": "vscode"}, "future": 42}); err != nil {
		t.Fatal(err)
	}
	if err := Update(path, map[string]any{"jira": map[string]string{"pattern": "KAKE-[0-9]{3,5}"}}); err != nil {
		t.Fatal(err)
	}
	doc, err := Read(path)
	if err != nil || doc["editors"] == nil || string(doc["future"]) != "42" {
		t.Fatal(doc, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("expected private config", err)
	}
}
