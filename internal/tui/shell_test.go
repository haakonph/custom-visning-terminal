package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/haakonph/custom-visning-terminal/internal/connectors/copilot"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/jira"
)

func TestShellRoutesBackgroundCopilotResultWhileGitHubIsActive(t *testing.T) {
	m := ready(t).WithCopilot(testCopilot(t), false)
	m.copilot.sessions = []copilot.Session{{ID: "old"}}
	m.copilotActive = false

	updated, _ := m.Update(copilotResult{err: errors.New("collector unavailable")})
	m = updated.(Model)
	if m.copilotActive || !strings.Contains(m.copilot.notice, "collector unavailable") {
		t.Fatalf("background Copilot result was not retained: %+v", m.copilot)
	}
	if len(m.copilot.sessions) != 1 || m.copilot.sessions[0].ID != "old" {
		t.Fatal("background failure discarded Copilot data")
	}
}

func TestShellRoutesBackgroundJiraResultWhileGitHubIsActive(t *testing.T) {
	m := jiraModel(t)
	defer m.cancel()
	m.jiraActive = false
	m.jira.generation = 3
	m.jira.rows = []jira.Issue{{Key: "APP-1"}}

	updated, _ := m.Update(jiraResult{generation: 3, rows: []jira.Issue{{Key: "APP-2"}}})
	m = updated.(Model)
	if m.jiraActive || len(m.jira.rows) != 1 || m.jira.rows[0].Key != "APP-2" {
		t.Fatalf("background Jira result was not routed: %+v", m.jira.rows)
	}
}
