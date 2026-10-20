package tui

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/local"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func localModel(t *testing.T) Model {
	t.Helper()
	m := ready(t)
	w, err := local.New(t.TempDir(), local.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	c := local.Defaults()
	binary, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git required")
	}
	c.Editors.IntelliJ.Command = binary
	return m.WithLocal(w, c.Editors)
}
func TestPRSelectionAcrossViews(t *testing.T) {
	m := localModel(t)
	m, _ = update(m, key(' '))
	m.snapshot.Datasets[1].PRs = []model.PullRequest{{Repository: "other/repo", Number: 7, URL: "https://github.com/other/repo/pull/7"}}
	m, _ = sequence(m, "l")
	m, _ = update(m, key(' '))
	if len(m.local.chosen) != 2 {
		t.Fatal("selection lost across views")
	}
	m, _ = sequence(m, "h")
	m, _ = update(m, key(' '))
	if len(m.local.chosen) != 1 {
		t.Fatal("toggle failed")
	}
}
func TestEditorChoiceAndFallback(t *testing.T) {
	m := localModel(t)
	m, _ = sequence(m, "ev")
	if !m.local.choosing {
		t.Fatal("no chooser")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.local.choosing {
		t.Fatal("chooser did not close")
	}
	m, cmd := sequence(m, "ee")
	if cmd == nil || m.local.running != 1 {
		t.Fatal("highlighted fallback not queued")
	}
	m, cmd = sequence(m, "ee")
	if cmd != nil {
		t.Fatal("duplicate operation queued")
	}
}
func TestPartialOpenResultsContinue(t *testing.T) {
	m := localModel(t)
	p := m.filtered()[0]
	q := m.filtered()[1]
	m.local.running = 1
	m.local.preparing[p.Key()] = true
	m.local.queue = []model.PullRequest{q}
	m, cmd := update(m, localResult{p: p, err: errors.New("access denied")})
	if cmd == nil || m.local.running != 1 || m.local.statuses[p.Key()].Error == "" {
		t.Fatal("failure stopped queue")
	}
	m, _ = update(m, localResult{p: q, status: local.Status{Path: "/tmp/pr"}})
	if m.local.running != 0 || len(m.local.results) != 2 {
		t.Fatal("missing per PR results")
	}
}
func TestStatusNoGitInRender(t *testing.T) {
	m := localModel(t)
	p := m.filtered()[0]
	m.local.statuses[p.Key()] = local.Status{Path: "/missing", Modified: true, Outdated: true}
	if got := m.localLabel(p); got != "Modified · Outdated" {
		t.Fatal(got)
	}
	m.local.preparing[p.Key()] = true
	if got := m.localLabel(p); got != "Preparing" {
		t.Fatal(got)
	}
	_ = m.View()
}

func TestProgressErrorsRemainVisible(t *testing.T) {
	m := localModel(t)
	p := m.filtered()[0]
	for i := 0; i < 30; i++ {
		m.snapshot.Datasets[0].PRs = append(m.snapshot.Datasets[0].PRs, p)
	}
	m.local.results = []string{"first result", "second result", "third result"}
	m.notice = "editor missing"
	m.warnings[m.active] = "GitHub partial response"
	content := m.View().Content
	for _, text := range []string{"first result", "third result", "editor missing", "GitHub partial response"} {
		if !strings.Contains(content, text) {
			t.Fatalf("%q hidden in full dashboard: %s", text, content)
		}
	}
}

func TestMainWarmQueueDeduplicatesRepositoriesAcrossViews(t *testing.T) {
	m := localModel(t)
	m.snapshot.Datasets[1].PRs = []model.PullRequest{{Repository: "other/repo", Number: 7}}
	if cmd := m.warmLocal(); cmd == nil || !m.local.warming || len(m.local.warmed) != 1 {
		t.Fatal("main preparation not scheduled")
	}
	if cmd := m.warmLocal(); cmd != nil {
		t.Fatal("duplicate main preparation scheduled")
	}
	m, cmd := update(m, localWarmResult{repository: "acme/api", err: errors.New("build failed")})
	if cmd == nil || !m.local.warming || len(m.local.warmed) != 2 || len(m.local.results) != 1 {
		t.Fatal("failure blocked the next repository")
	}
	m, cmd = update(m, localWarmResult{repository: "other/repo"})
	if cmd != nil || m.local.warming {
		t.Fatal("warmed repositories should wait before refreshing")
	}
	m.local.manager.Workspace.Warm = false
	m.snapshot.Datasets[2].PRs = []model.PullRequest{{Repository: "third/repo"}}
	if cmd := m.warmLocal(); cmd != nil {
		t.Fatal("disabled warming scheduled a job")
	}
}

func TestBackgroundPreparationCompletesWhileSettingsOpen(t *testing.T) {
	m := localModel(t)
	m.settings.active = true
	m.local.warming = true
	m.local.manager.Workspace.Warm = false
	m, _ = update(m, localWarmResult{repository: "acme/api"})
	if m.local.warming || !strings.Contains(m.local.warmNotice, "Main klar") || !m.settings.active {
		t.Fatal("settings swallowed background result")
	}
}

func TestMainWarmQueuePrefersUnpreparedRepository(t *testing.T) {
	m := localModel(t)
	m.local.warmed["acme/api"] = time.Now().Add(-10 * time.Minute)
	m.snapshot.Datasets[1].PRs = []model.PullRequest{{Repository: "other/repo"}}
	if cmd := m.warmLocal(); cmd == nil || m.local.warmed["other/repo"].IsZero() {
		t.Fatal("slow preparation starved a new repository")
	}
}
