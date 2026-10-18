package jira

import (
	appconfig "github.com/haakonph/custom-visning-terminal/internal/config"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateLegacyConfigOnce(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CUSTOM_VISNING_CONFIG", filepath.Join(dir, "config.json"))
	legacy := filepath.Join(dir, "jira.json")
	t.Setenv("JIRA_CONFIG", legacy)
	original := []byte(`{"pattern":"KAKE-[0-9]{3,5}","queries":[{"name":"Mine","jql":"assignee = currentUser()"}]}`)
	if err := os.WriteFile(legacy, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := appconfig.Update(ConfigPath(), map[string]any{"editors": map[string]string{"default": "vscode"}}); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(); err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig(ConfigPath())
	if err != nil || config.Pattern != "KAKE-[0-9]{3,5}" || len(config.Queries) != 1 {
		t.Fatal(config, err)
	}
	config.Pattern = "OTHER-[0-9]+"
	if err := SaveConfig(ConfigPath(), config); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(); err != nil {
		t.Fatal(err)
	}
	config, _ = LoadConfig(ConfigPath())
	if config.Pattern != "OTHER-[0-9]+" {
		t.Fatal("legacy config overwrote new settings")
	}
	got, _ := os.ReadFile(legacy)
	if string(got) != string(original) {
		t.Fatal("original changed")
	}
	doc, _ := appconfig.Read(ConfigPath())
	if doc["editors"] == nil {
		t.Fatal("editors lost")
	}
}

func TestInvalidLegacyConfigIsPreserved(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CUSTOM_VISNING_CONFIG", filepath.Join(dir, "config.json"))
	legacy := filepath.Join(dir, "jira.json")
	t.Setenv("JIRA_CONFIG", legacy)
	if err := os.WriteFile(legacy, []byte(`{"pattern":"["}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(); err == nil {
		t.Fatal("accepted invalid legacy pattern")
	}
	if _, err := os.Stat(ConfigPath()); !os.IsNotExist(err) {
		t.Fatal("wrote config despite migration failure")
	}
}
