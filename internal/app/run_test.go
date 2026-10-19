package app

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestInvalidJiraPatternFailsBeforeStartup(t *testing.T) {
	t.Setenv("CUSTOM_VISNING_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	t.Setenv("JIRA_CONFIG", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("JIRA_PATTERN", "[")
	err := RunProviders(context.Background(), io.Discard, false, false, "dev")
	if err == nil || !strings.Contains(err.Error(), "invalid JIRA_PATTERN") {
		t.Fatalf("expected pattern error, got %v", err)
	}
}
