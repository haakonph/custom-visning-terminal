package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/local"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestExistingWorktreeLaunchesBeforeProjectPreparation(t *testing.T) {
	m := localModel(t)
	p := m.filtered()[0]
	path := filepath.Join(m.local.manager.Repositories.Dir, "worktrees", strings.ToLower(p.Repository), fmt.Sprintf("pr-%d", p.Number))
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init"},
		{"-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "initial"},
		{"remote", "add", "origin", "https://github.com/" + p.Repository + ".git"},
	} {
		if output, err := exec.Command("git", append([]string{"-C", path}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, output, err)
		}
	}
	// No preparation is needed to launch an existing checkout, even when it
	// contains a Maven project and Maven is unavailable.
	if err := os.WriteFile(filepath.Join(path, "pom.xml"), []byte("<project/>"), 0600); err != nil {
		t.Fatal(err)
	}
	m.local.launcher.Editors.IntelliJ.Args = []string{"--version"}
	m.local.queue = []model.PullRequest{p}
	msg := m.nextLocal()()
	opened, ok := msg.(localOpened)
	if !ok || opened.path != path {
		t.Fatalf("expected editor launch before preparation, got %#v", msg)
	}
}

type closureSource struct{ fakeSource }

func (*closureSource) Closed(context.Context, model.PullRequest) (bool, error) {
	return false, nil
}

func TestIntelliJCleanupRunsWithoutMainWarming(t *testing.T) {
	m := localModel(t)
	m.useSource(&closureSource{})
	m.local.manager.Workspace.Warm = false
	if cmd := m.refreshLocal(); cmd == nil || !m.local.warmTimerStarted || !m.local.cleanupRunning {
		t.Fatal("startup did not schedule cleanup independently of main warming")
	}
	if cmd := m.cleanupIntelliJ(true); cmd != nil {
		t.Fatal("overlapping cleanup scheduled")
	}
	m.settings.active = true
	m, _ = update(m, localCleanupResult{err: errors.New("offline")})
	if m.local.cleanupRunning || m.local.cleanupAt.IsZero() || !m.settings.active || len(m.local.results) != 1 {
		t.Fatal("settings swallowed cleanup result")
	}
	if cmd := m.cleanupIntelliJ(false); cmd != nil {
		t.Fatal("background cleanup ignored interval")
	}
	if cmd := m.cleanupIntelliJ(true); cmd == nil {
		t.Fatal("refresh did not retry cleanup")
	}
	m, _ = update(m, localCleanupResult{})
	m.local.cleanupAt = time.Now().Add(-6 * time.Minute)
	m, cmd := update(m, localWarmTick{})
	if cmd == nil || !m.local.cleanupRunning || m.local.warming {
		t.Fatal("background timer did not clean up with main warming disabled")
	}
}

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
	m.editPRs(func(snapshot *model.Snapshot) {
		snapshot.Datasets[1].PRs = []model.PullRequest{{Repository: "other/repo", Number: 7, URL: "https://github.com/other/repo/pull/7"}}
	})
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
		m.editPRs(func(snapshot *model.Snapshot) { snapshot.Datasets[0].PRs = append(snapshot.Datasets[0].PRs, p) })
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

func TestOpenedWorktreeUpdatesInBackgroundAndContinuesQueue(t *testing.T) {
	m := localModel(t)
	p, q := m.filtered()[0], m.filtered()[1]
	m.local.running = 1
	m.local.preparing[p.Key()] = true
	m.local.preparing[q.Key()] = true
	m.local.queue = []model.PullRequest{q}
	m, cmd := update(m, localOpened{p: p, path: "/tmp/existing"})
	if cmd == nil || m.local.running != 1 || len(m.local.queue) != 0 {
		t.Fatal("background update blocked next editor")
	}
	if m.localLabel(p) != "Opened · Updating" || !strings.Contains(m.View().Content, "1 updating") {
		t.Fatal("background update is not visible")
	}
	if m.openLocal("") != nil {
		t.Fatal("duplicate open accepted during background update")
	}
	m, _ = update(m, localResult{p: p, status: local.Status{Path: "/tmp/existing"}, err: errors.New("offline"), updated: true})
	if m.local.running != 1 || m.local.preparing[p.Key()] || m.local.updating[p.Key()] {
		t.Fatal("update completion changed another opening job")
	}
	if !strings.Contains(m.notice, "opened; background update failed: offline") {
		t.Fatal(m.notice)
	}
}

func TestMainWarmQueueDeduplicatesRepositoriesAcrossViews(t *testing.T) {
	m := localModel(t)
	m.editPRs(func(snapshot *model.Snapshot) {
		snapshot.Datasets[1].PRs = []model.PullRequest{{Repository: "other/repo", Number: 7}}
	})
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
	m.editPRs(func(snapshot *model.Snapshot) {
		snapshot.Datasets[2].PRs = []model.PullRequest{{Repository: "third/repo"}}
	})
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
	m.editPRs(func(snapshot *model.Snapshot) {
		snapshot.Datasets[1].PRs = []model.PullRequest{{Repository: "other/repo"}}
	})
	if cmd := m.warmLocal(); cmd == nil || m.local.warmed["other/repo"].IsZero() {
		t.Fatal("slow preparation starved a new repository")
	}
}

func TestLocalFailuresNotifyWhileSettingsOpen(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		note    string
		updated bool
		want    bool
	}{
		{"first preparation fails", errors.New("clone failed"), "", false, true},
		{"background update fails", errors.New("offline"), "", true, true},
		{"build fails in status", nil, "npm preparation failed", true, true},
		{"unsafe update preserved", nil, "local commits or rewritten history; update manually", true, true},
		{"success", nil, "", true, false},
		{"indexing not configured", nil, "headless indexing not configured", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := localModel(t)
			defer m.cancel()
			m.settings.active = true
			m.notifications.options = notificationOptions{}
			p := m.filtered()[0]
			m.local.preparing[p.Key()] = true
			m.local.running = 1
			if tc.updated {
				m.local.running = 0
				m.local.updating[p.Key()] = true
			}
			m, cmd := update(m, localResult{p: p, status: local.Status{Path: "/tmp/existing", Note: tc.note}, err: tc.err, updated: tc.updated})
			if (cmd != nil) != tc.want {
				t.Fatalf("notification scheduled=%v, want %v", cmd != nil, tc.want)
			}
			if !m.settings.active || m.local.running != 0 || m.local.preparing[p.Key()] || m.local.updating[p.Key()] {
				t.Fatal("result interrupted settings or left an opening job pending")
			}
		})
	}
}
