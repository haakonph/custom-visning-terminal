package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

type fakeSource struct {
	result model.Result
	err    error
	calls  int
}

func (f *fakeSource) User(context.Context) (string, error) { return "alice", f.err }
func (f *fakeSource) List(ctx context.Context, v model.View, user string) (model.Result, error) {
	f.calls++
	return f.result, f.err
}

type fakeCache struct {
	snap  model.Snapshot
	err   error
	saves int
}

func (f *fakeCache) Load(string) (model.Snapshot, error) { return f.snap, f.err }
func (f *fakeCache) Save(s model.Snapshot) error         { f.snap = s; f.saves++; return f.err }
func key(code rune) tea.KeyPressMsg                      { return tea.KeyPressMsg{Code: code, Text: string(code)} }
func update(m Model, msg tea.Msg) (Model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}
func ready(t *testing.T) Model {
	t.Helper()
	t.Setenv("CUSTOM_VISNING_LINKS", t.TempDir()+"/links.sqlite")
	t.Setenv("CUSTOM_VISNING_TASKS", t.TempDir()+"/tasks.sqlite")
	m := New(context.Background(), &fakeSource{}, nil)
	t.Cleanup(m.cancel)
	m.booting = false
	m.snapshot = model.Snapshot{Version: 1, User: "alice"}
	m.snapshot.Datasets[0].PRs = []model.PullRequest{{URL: "https://github.com/acme/api/pull/1", Title: "Fix login", Repository: "acme/api", Author: "alice", Number: 1}, {URL: "https://github.com/acme/api/pull/2", Title: "Add endpoint", Repository: "acme/api", Author: "bob", Number: 2, RequestedTeams: []string{"acme/core"}, Labels: []string{"feature"}}}
	return m
}
func TestNavigationAndFilter(t *testing.T) {
	m := ready(t)
	m, _ = update(m, key('j'))
	if m.selected != 1 {
		t.Fatal("j")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyUp})
	if m.selected != 0 {
		t.Fatal("up")
	}
	m, _ = update(m, key('k'))
	if m.selected != 0 {
		t.Fatal("underflow")
	}
	m, _ = update(m, key('/'))
	if !m.filtering {
		t.Fatal("filter not focused")
	}
	for _, r := range "endpoint" {
		m, _ = update(m, key(r))
	}
	if rows := m.filtered(); len(rows) != 1 || rows[0].Number != 2 {
		t.Fatalf("filter: %+v", rows)
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.filtering || m.input.Value() != "endpoint" {
		t.Fatal("filter not retained")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if len(m.filtered()) != 2 {
		t.Fatal("filter not cleared")
	}
	for _, filter := range []string{"BOB", "feature", "acme/core"} {
		m.input.SetValue(filter)
		if len(m.filtered()) != 1 {
			t.Fatalf("filter %s", filter)
		}
	}
	for _, r := range "1234" {
		m, _ = sequence(m, "l")
		if m.active != model.View((r-'0')%4) {
			t.Fatal("switch view")
		}
	}
}
func TestRefreshPreservesCacheAndSyncTime(t *testing.T) {
	m := ready(t)
	at := time.Now().Add(-time.Hour)
	m.snapshot.Datasets[0].SyncedAt = at
	m, _ = update(m, refreshMsg{view: 0, result: model.Result{PRs: []model.PullRequest{{URL: "new", Title: "New"}}, Complete: false}, err: errors.New("network error")})
	if len(m.filtered()) != 3 || !m.snapshot.Datasets[0].SyncedAt.Equal(at) || m.warnings[0] == "" {
		t.Fatal("partial refresh lost cache or sync time")
	}
	fresh := time.Now()
	m, _ = update(m, refreshMsg{view: 0, result: model.Result{Complete: true}, at: fresh})
	if len(m.filtered()) != 0 || !m.snapshot.Datasets[0].SyncedAt.Equal(fresh) || m.warnings[0] != "" {
		t.Fatal("complete empty refresh should remove old rows")
	}
}
func TestRefreshAsyncAndNoDuplicates(t *testing.T) {
	m := ready(t)
	source := &fakeSource{result: model.Result{Complete: true}}
	m.source = source
	var cmd tea.Cmd
	m, cmd = sequence(m, "au")
	if cmd == nil || source.calls != 0 || !m.loading[0] {
		t.Fatal("refresh blocked Update")
	}
	next, duplicate := sequence(m, "au")
	m = next
	if duplicate != nil {
		t.Fatal("duplicate refresh")
	}
	msg := cmd()
	if source.calls != 1 {
		t.Fatal("source not called")
	}
	m, _ = update(m, msg)
	if m.loading[0] {
		t.Fatal("still loading")
	}
}
func TestBootstrapLoadsCacheBeforeRefresh(t *testing.T) {
	store := &fakeCache{snap: model.Snapshot{Version: 1, User: "alice"}}
	store.snap.Datasets[0].PRs = []model.PullRequest{{Title: "cached"}}
	source := &fakeSource{}
	m := New(context.Background(), source, store)
	defer m.cancel()
	msg := m.bootstrap()()
	m, cmd := update(m, msg)
	if cmd == nil || len(m.filtered()) != 1 || source.calls != 0 || m.snapshot.User != "alice" {
		t.Fatal("cache was not displayed before API refresh")
	}
	for _, loading := range m.loading {
		if !loading {
			t.Fatal("missing initial refresh")
		}
	}
}
func TestBootstrapErrorAndRetry(t *testing.T) {
	source := &fakeSource{err: errors.New("auth missing")}
	m := New(context.Background(), source, nil)
	defer m.cancel()
	m, _ = update(m, m.bootstrap()())
	if m.notice != "auth missing" || m.booting {
		t.Fatal("auth error missing")
	}
	source.err = nil
	m, cmd := sequence(m, "au")
	if !m.booting || cmd == nil {
		t.Fatal("cannot retry bootstrap")
	}
}
func TestCacheWritesCoalesce(t *testing.T) {
	m := ready(t)
	store := &fakeCache{}
	m.cache = store
	m, first := update(m, refreshMsg{view: 0, result: model.Result{Complete: true, PRs: []model.PullRequest{{Title: "first"}}}})
	m, second := update(m, refreshMsg{view: 0, result: model.Result{Complete: true, PRs: []model.PullRequest{{Title: "latest"}}}})
	if first == nil || second != nil || !m.dirty {
		t.Fatal("writes not coalesced")
	}
	m, next := update(m, first())
	if next == nil {
		t.Fatal("missing latest snapshot write")
	}
	m, _ = update(m, next())
	if store.snap.Datasets[0].PRs[0].Title != "latest" || m.saving || store.saves != 2 {
		t.Fatal("stale disk state")
	}
}
func TestOpenAndQuit(t *testing.T) {
	m := ready(t)
	opened := ""
	m.open = func(ctx context.Context, url string) error { opened = url; return nil }
	_, cmd := sequence(m, "en")
	if cmd == nil || opened != "" {
		t.Fatal("browser opened synchronously")
	}
	cmd()
	if opened != m.filtered()[0].URL {
		t.Fatal("wrong PR opened")
	}
	m, cmd = update(m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil || m.ctx.Err() == nil {
		t.Fatal("quit did not cancel")
	}
	if err := openBrowser(context.Background(), "https://evil.example/"); err == nil {
		t.Fatal("unsafe URL accepted")
	}
}
func TestResponsiveViewAndHelp(t *testing.T) {
	m := ready(t)
	for _, size := range [][2]int{{120, 30}, {80, 15}, {40, 10}, {8, 3}, {1, 1}} {
		m, _ = update(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := m.View()
		if !view.AltScreen {
			t.Fatal("not using v2 alt screen")
		}
		lines := strings.Split(view.Content, "\n")
		if len(lines) > size[1] {
			t.Fatal("vertical overflow")
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("horizontal overflow: %q", line)
			}
		}
	}
	m, _ = update(m, tea.WindowSizeMsg{Width: 120, Height: 30})
	m, _ = update(m, key('?'))
	if !strings.Contains(m.View().Content, "Hurtigtaster · custom-visning · Versjon dev") {
		t.Fatal("help or development version missing")
	}
	m = m.WithVersion("main-42.1-abcdef123456")
	if !strings.Contains(m.View().Content, "Versjon main-42.1-abcdef123456") {
		t.Fatal("build version missing from help")
	}
}

func TestOfflineStartupUsesCredentialScopedCache(t *testing.T) {
	store := &fakeCache{snap: model.Snapshot{Version: 1, User: "alice"}}
	store.snap.Datasets[0].PRs = []model.PullRequest{{Title: "cached offline", URL: "cached"}}
	source := &fakeSource{err: errors.New("offline")}
	m := New(context.Background(), source, store)
	defer m.cancel()
	msg := m.bootstrap()().(bootMsg)
	if msg.err != nil || msg.snapshot.User != "alice" || len(msg.snapshot.Datasets[0].PRs) != 1 {
		t.Fatalf("offline cache unavailable: %+v", msg)
	}
}

func TestHelpScrollAndSmallWindows(t *testing.T) {
	m := snoozeReady(t).WithVersion("main-42.1-abcdef123456")
	m = press(m, "?")
	for _, size := range [][2]int{{120, 30}, {40, 10}, {8, 3}, {1, 1}} {
		m, _ = update(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for range 200 {
			m = press(m, "j")
		}
		view := m.View()
		if !view.AltScreen || len(strings.Split(view.Content, "\n")) > size[1] {
			t.Fatal("help exceeds window height")
		}
		for _, line := range strings.Split(view.Content, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatal("help exceeds window width")
			}
		}
	}
	m, _ = update(m, tea.WindowSizeMsg{Width: 120, Height: 10})
	if !strings.Contains(m.View().Content, "Delvis oppdatering") {
		t.Fatal("cannot reach the end of help")
	}
	for range 200 {
		m = press(m, "k")
	}
	if !strings.Contains(m.View().Content, "Hurtigtaster") {
		t.Fatal("cannot return to the start of help")
	}
	m = press(m, "zZ")
	if m.snoozes.input || m.snoozes.active {
		t.Fatal("help keys changed snoozes")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.help {
		t.Fatal("Esc did not close help")
	}
}
