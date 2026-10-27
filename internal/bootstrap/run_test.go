package bootstrap

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenUsageDisabledDoesNotCreateDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.sqlite")
	t.Setenv("CUSTOM_VISNING_METRICS", path)
	t.Setenv("CUSTOM_VISNING_METRICS_DISABLED", "1")
	r, notice := openUsage(io.Discard)
	if r != nil || !strings.Contains(notice, "Deaktivert") {
		t.Fatalf("disabled metrics: %v %q", r, notice)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("disabled metrics created database: %v", err)
	}
}

func TestOpenUsageUnavailableDoesNotStopApp(t *testing.T) {
	t.Setenv("CUSTOM_VISNING_METRICS", t.TempDir())
	t.Setenv("CUSTOM_VISNING_METRICS_DISABLED", "0")
	r, notice := openUsage(io.Discard)
	if r != nil || notice == "" {
		t.Fatalf("unavailable metrics: %v %q", r, notice)
	}
}

func TestInvalidJiraPatternFailsBeforeStartup(t *testing.T) {
	t.Setenv("CUSTOM_VISNING_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	t.Setenv("JIRA_CONFIG", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("JIRA_PATTERN", "[")
	err := RunProviders(context.Background(), io.Discard, false, false, "dev")
	if err == nil || !strings.Contains(err.Error(), "invalid JIRA_PATTERN") {
		t.Fatalf("expected pattern error, got %v", err)
	}
}
