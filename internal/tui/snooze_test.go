package tui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/jira"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/snooze"
)

func snoozeReady(t *testing.T) Model {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CUSTOM_VISNING_SNOOZE", filepath.Join(dir, "snooze.json"))
	t.Setenv("CUSTOM_VISNING_TASKS", filepath.Join(dir, "tasks.json"))
	return ready(t)
}

func press(m Model, text string) Model {
	for _, r := range text { m, _ = update(m, key(r)) }
	return m
}

func TestSnoozeInputAndEscape(t *testing.T) {
	m := snoozeReady(t)
	m = press(m, "z12")
	if !m.snoozes.input || m.snoozes.digits != "12" { t.Fatal("multi digit input") }
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if m.snoozes.digits != "1" { t.Fatal("backspace") }
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.snoozes.input || len(m.snoozes.entries) != 0 { t.Fatal("escape changed state") }
	if _, err := os.Stat(m.snoozes.store.Path); !os.IsNotExist(err) { t.Fatal("escape wrote a file") }
	m = press(m, "z0h")
	if !m.snoozes.input || len(m.filtered()) != 2 { t.Fatal("zero accepted") }
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m = press(m, "z9999999999999999999999999D")
	if !m.snoozes.input || len(m.snoozes.entries) != 0 { t.Fatal("overflow accepted") }
}

func TestSnoozeCrossViewRestartAndAutomaticExpiry(t *testing.T) {
	m := snoozeReady(t)
	now := time.Now()
	m.snoozes.now = func() time.Time { return now }
	pr := m.filtered()[0]
	m.snapshot.Datasets[model.Reviews].PRs = []model.PullRequest{pr}
	m = press(m, "z12h")
	if m.snoozes.input || len(m.filtered()) != 1 || m.selected != 0 { t.Fatal("row not hidden naturally") }
	m.active = model.Reviews
	if len(m.filtered()) != 0 { t.Fatal("PR visible in other tab") }
	m.input.SetValue("login")
	if len(m.filtered()) != 0 { t.Fatal("PR visible in search") }
	restarted := New(context.Background(), nil, nil)
	t.Cleanup(restarted.cancel)
	if len(restarted.snoozes.rows()) != 1 { t.Fatal("snooze lost after restart") }
	m = press(m, "Zz2h")
	if rows := m.snoozes.rows(); len(rows) != 1 || !rows[0].Until.Equal(now.Add(2*time.Hour)) { t.Fatal("replacement not based on now") }
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	now = now.Add(2*time.Hour)
	m, cmd := update(m, snoozeTick{})
	if cmd == nil || len(m.filtered()) != 1 { t.Fatal("no automatic expiry") }
	if rows := m.snoozes.rows(); len(rows) != 0 { t.Fatal("expired overview row") }
}

func TestSnoozeEditingAndWriteFailure(t *testing.T) {
	m := snoozeReady(t)
	m = press(m, "/z12hZ")
	if m.snoozes.input || m.snoozes.active || len(m.snoozes.entries) != 0 { t.Fatal("snooze consumed text") }
	m.filtering = false
	m.input.SetValue("")
	m.snoozes.store.Path = t.TempDir() // rename onto a directory must fail
	m = press(m, "z1h")
	if !m.snoozes.input || len(m.filtered()) != 2 || len(m.snoozes.entries) != 0 { t.Fatal("failed write presented as saved") }
}

func TestJiraAndTaskScope(t *testing.T) {
	m := snoozeReady(t)
	now := m.snoozes.at
	link := "https://jira.example/browse/ABC-1"
	id := snooze.Identity("jira", link)
	m.snoozes.entries[id] = snooze.Entry{ID: id, Kind: "jira", Title: "Issue", Until: now.Add(time.Hour)}
	m.jira = &jiraState{snoozes: m.snoozes, rows: []jira.Issue{{URL: link}, {URL: "https://other.example/browse/ABC-1"}}}
	if len(m.jira.filtered()) != 1 { t.Fatal("Jira server identity") }
	m.tasks.items = []model.Task{{Title: "Task", References: []model.TaskReference{{Provider: "jira", URL: link, Title: "Issue"}}}}
	m.tasks.active = true
	m = press(m, "z")
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.tasks.items[0].ID != "" { t.Fatal("Esc changed task") }
	m = press(m, "z1h")
	if len(m.tasks.visibleTasks()) != 0 || len(m.snoozes.entries) != 2 { t.Fatal("task scope") }
	if len(m.tasks.items[0].References) != 1 { t.Fatal("reference removed") }
	m = press(m, "Z")
	for i, e := range m.snoozes.rows() { if e.Kind == "task" { m.snoozes.selected = i } }
	m = press(m, "u")
	if len(m.tasks.visibleTasks()) != 1 || !m.snoozes.hidden(id) { t.Fatal("restoring task restored reference") }
	restarted := New(context.Background(), nil, nil)
	t.Cleanup(restarted.cancel)
	if restarted.tasks.items[0].ID != m.tasks.items[0].ID { t.Fatal("task identity changed on restart") }
}

func TestExpiryPreservesHighlightedRow(t *testing.T) {
	m := snoozeReady(t)
	now := time.Now()
	m.snoozes.now = func() time.Time { return now }
	m = press(m, "z1h")
	selected := m.filtered()[m.selected].URL
	now = now.Add(time.Hour)
	m, _ = update(m, snoozeTick{})
	if len(m.filtered()) != 2 || m.filtered()[m.selected].URL != selected { t.Fatal("expiry moved highlight") }
}

func TestSnoozeKeysStayTextInOtherEditors(t *testing.T) {
	m := snoozeReady(t)
	m.tasks.active, m.tasks.editing = true, true
	m.tasks.items = []model.Task{{Title: "Task"}}
	m.tasks.input.Focus()
	m = press(m, "z12hZ")
	if m.snoozes.input || m.snoozes.active || m.tasks.input.Value() != "z12hZ" { t.Fatal("task editor consumed snooze keys") }
	m.tasks.active = false
	m.jiraActive = true
	m.jira = &jiraState{mode: "jql", input: m.input, snoozes: m.snoozes}
	m.jira.input.Focus()
	m = press(m, "z14DZ")
	if m.snoozes.input || m.snoozes.active || m.jira.input.Value() != "z14DZ" { t.Fatal("JQL editor consumed snooze keys") }
}
