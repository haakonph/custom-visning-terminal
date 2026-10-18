package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/copilot"
)

func TestCopilotNavigationFilteringAndRetention(t *testing.T) {
	client := testCopilot(t)
	m := ready(t).WithCopilot(client, false)
	m, cmd := update(m, key('c'))
	if !m.copilotActive || cmd == nil {
		t.Fatal("Copilot not opened")
	}
	m, cmd = update(m, copilotResult{sessions: []copilot.Session{{ID: "s1", Name: "Fix login", Repository: "acme/api"}, {ID: "s2", Name: "Review", Jira: "TEST-12"}}})
	if cmd == nil {
		t.Fatal("poll not scheduled")
	}
	m, _ = update(m, key('/'))
	for _, r := range "TEST-12" {
		m, _ = update(m, key(r))
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(m.copilot.rows()) != 1 || m.copilot.rows()[0].ID != "s2" {
		t.Fatal("filter failed")
	}
	m, cmd = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || m.copilot.detail.Session.ID != "s2" {
		t.Fatal("detail not loaded")
	}
	m, _ = update(m, copilotResult{id: "s2", err: errors.New("offline")})
	if len(m.copilot.sessions) != 2 || !strings.Contains(m.copilot.notice, "offline") {
		t.Fatal("failed refresh lost data or error")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.copilot.detail != nil || len(m.copilot.rows()) != 2 {
		t.Fatal("escape did not reset")
	}
	m, _ = update(m, key('g'))
	if m.copilotActive {
		t.Fatal("GitHub switch failed")
	}
}
func TestStandaloneCopilotAndSafeRendering(t *testing.T) {
	client := testCopilot(t)
	m := New(context.Background(), nil, nil).WithCopilot(client, true)
	defer m.cancel()
	if m.Init() == nil {
		t.Fatal("standalone init")
	}
	m, _ = update(m, key('g'))
	if !m.copilotActive || m.copilot.notice == "" {
		t.Fatal("missing provider feedback")
	}
	m.copilot.detail = &copilot.Detail{Session: copilot.Session{Name: "Session"}, Events: []copilot.Event{{Type: "tool.complete", Content: "line1\nline2\x1b]52;secret\a", Args: "args", Result: "result"}}}
	m.width = 35
	m.height = 10
	rendered := m.View().Content
	if strings.Contains(rendered, "\x1b]52") || len(strings.Split(rendered, "\n")) > m.height {
		t.Fatal("unsafe or oversized rendering")
	}
	m, _ = update(m, key('d'))
	if !m.copilot.diagnosticsOpen {
		t.Fatal("diagnostics toggle")
	}
}

func testCopilot(t *testing.T) *copilot.Service {
	t.Helper()
	dir := t.TempDir()
	client, err := copilot.New(copilot.Options{SessionRoot: dir + "/sessions", StorePath: dir + "/store.json", JiraPattern: `[A-Z]+-[0-9]+`, CaptureContent: true, ShowContent: true})
	if err != nil {
		t.Fatal(err)
	}
	return client
}
