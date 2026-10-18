package app

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestInvalidJiraPatternFailsBeforeStartup(t *testing.T) {
	t.Setenv("JIRA_PATTERN", "[")
	err := RunProviders(context.Background(), io.Discard, false, false)
	if err == nil || !strings.Contains(err.Error(), "invalid JIRA_PATTERN") {
		t.Fatalf("expected pattern error, got %v", err)
	}
}
