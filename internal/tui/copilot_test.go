package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/copilot"
)

func TestCopilotNavigationFilteringAndRetention(t *testing.T) {
	client := testCopilot(t)
	m := ready(t).WithCopilot(client, false)
	m, cmd := sequence(m, "va")
	if !(m.page == pageCopilot) || cmd == nil {
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
	m, cmd = sequence(m, "d")
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
	m, _ = sequence(m, "vg")
	if m.page == pageCopilot {
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
	m, _ = sequence(m, "vg")
	if !(m.page == pageCopilot) || m.setup.provider == "" {
		t.Fatal("missing provider feedback")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m.copilot.detail = &copilot.Detail{Session: copilot.Session{Name: "Session"}, Events: []copilot.Event{{Type: "tool.complete", Content: "line1\nline2\x1b]52;secret\a", Args: "args", Result: "result"}}}
	m.width = 35
	m.height = 10
	rendered := m.View().Content
	if strings.Contains(rendered, "\x1b]52") || len(strings.Split(rendered, "\n")) > m.height {
		t.Fatal("unsafe or oversized rendering")
	}
	m, _ = sequence(m, "vi")
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

func TestCopilotDiagnosticsExporterConfig(t *testing.T) {
	m := ready(t).WithCopilot(testCopilot(t), true)
	m.width, m.height = 120, 40
	m.copilot.diagnostics = copilot.Diagnostics{Port: 14318}
	m, _ = sequence(m, "vi")
	for _, want := range []string{"http://127.0.0.1:14318", "http/json", "http/protobuf", "grpc støttes ikke", "/v1/traces", "/v1/logs", "/v1/metrics", "y kopierer"} {
		if !strings.Contains(m.View().Content, want) {
			t.Errorf("diagnostics missing %q", want)
		}
	}
	want := "export OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:14318\nexport OTEL_EXPORTER_OTLP_PROTOCOL=http/json"
	if got := m.copilot.otlpExporterConfig(); got != want {
		t.Fatalf("export config = %q, want %q", got, want)
	}
	m, cmd := update(m, key('y'))
	if cmd == nil || !strings.Contains(m.copilot.notice, "Kopiering sendt") {
		t.Fatal("diagnostics copy shortcut did not issue clipboard command")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	_, cmd = update(m, key('y'))
	if cmd != nil {
		t.Fatal("copy shortcut active outside diagnostics")
	}
}
