package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/copilot"
	"github.com/haakonph/custom-visning-terminal/internal/jira"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/storage"
)

// These tests deliberately use Update directly: I/O must stay in commands,
// whereas the shared interaction helper drains local completions for fixtures.
func TestLocalWritesDoNotBlockNavigationAndKeepOrder(t *testing.T) {
	m := ready(t)
	started, release := make(chan struct{}), make(chan struct{})
	var order []int
	m.enqueueLocal(func() error { close(started); <-release; order = append(order, 1); return nil }, func(*Model, error) {})
	m.enqueueLocal(func() error { order = append(order, 2); return nil }, func(*Model, error) {})
	next, write := m.Update(struct{}{})
	m = next.(Model)
	completed := make(chan tea.Msg, 1)
	go func() { completed <- write() }()
	<-started
	next, cmd := m.Update(key('j'))
	m = next.(Model)
	if m.selected != 1 || cmd != nil {
		t.Fatal("navigation must continue while the writer is blocked")
	}
	select {
	case <-completed:
		t.Fatal("writer unexpectedly completed")
	default:
	}
	close(release)
	next, write = m.Update(<-completed)
	m = next.(Model)
	if !reflect.DeepEqual(order, []int{1}) {
		t.Fatal(order)
	}
	next, _ = m.Update(write())
	m = next.(Model)
	if !reflect.DeepEqual(order, []int{1, 2}) || m.localIO.active != nil {
		t.Fatal(order)
	}
}

func TestTaskWriteOwnsItsSnapshot(t *testing.T) {
	m := ready(t)
	m.tasks.items = []model.Task{{ID: "one", Title: "Original", References: []model.TaskReference{{Title: "Original reference"}}}}
	if err := m.tasks.save(); err != nil {
		t.Fatal(err)
	}
	m.tasks.items[0].Title = "Changed"
	m.tasks.items[0].References[0].Title = "Changed reference"
	next, write := m.Update(struct{}{})
	next, _ = next.(Model).Update(write())
	m = next.(Model)
	var saved []model.Task
	if err := storage.Load(m.tasks.path, &saved); err != nil {
		t.Fatal(err)
	}
	if saved[0].Title != "Original" || saved[0].References[0].Title != "Original reference" {
		t.Fatal("queued snapshot changed with UI data", saved)
	}
}

func TestArchiveAppliesOnlyAfterSuccessfulWrite(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			m := ready(t)
			if fail {
				parent := filepath.Join(t.TempDir(), "file")
				if err := os.WriteFile(parent, nil, 0600); err != nil {
					t.Fatal(err)
				}
				m.tasks.path = filepath.Join(parent, "local.sqlite")
			}
			refs := []model.TaskReference{prRef(m.filtered()[0])}
			next, _ := m.archiveReferences(refs)
			m = next.(Model)
			if len(m.entities.data.Archived) != 0 {
				t.Fatal("archive changed before persistence")
			}
			next, write := m.Update(struct{}{})
			next, _ = next.(Model).Update(write())
			m = next.(Model)
			if fail {
				if len(m.entities.data.Archived) != 0 || !strings.Contains(m.sequenceNotice, "Kunne ikke") {
					t.Fatal("failure hid the item", m.sequenceNotice)
				}
			} else if len(m.entities.data.Archived) != 1 {
				t.Fatal("successful completion not applied")
			}
		})
	}
}

func TestQuitDrainsWritesAndAbortsOnWriteFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			m := ready(t)
			m.enqueueLocal(func() error {
				if fail {
					return errors.New("disk full")
				}
				return nil
			}, func(m *Model, err error) {
				if err != nil {
					m.notice = err.Error()
				}
			})
			next, write := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
			m = next.(Model)
			if m.ctx.Err() != nil || !m.localIO.quitting {
				t.Fatal("quit canceled pending persistence")
			}
			next, quit := m.Update(write())
			m = next.(Model)
			if fail {
				if m.ctx.Err() != nil || m.localIO.quitting || m.notice != "disk full" {
					t.Fatal("write failure must keep app open", m.notice)
				}
			} else if m.ctx.Err() == nil || quit == nil {
				t.Fatal("successful flush did not finish quit")
			}
		})
	}
}

func TestDetailScrollReusesWrappingAndResizeRebuildsIt(t *testing.T) {
	m := ready(t)
	m.detail = &detailState{data: model.PRDetails{Comments: []model.PRComment{{Body: strings.Repeat("long comment ", 1000)}}}}
	_ = m.View()
	before := m.detail.lines
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(Model)
	_ = m.View()
	if &before[0] != &m.detail.lines[0] {
		t.Fatal("scroll rebuilt wrapped history")
	}
	next, _ = m.Update(tea.WindowSizeMsg{Width: 40, Height: 12})
	m = next.(Model)
	_ = m.View()
	if &before[0] == &m.detail.lines[0] || len(m.detail.lines) <= len(before) {
		t.Fatal("resize retained old line wrapping")
	}
}

func TestGitHubNavigationReusesGroupsAndRefreshInvalidatesThem(t *testing.T) {
	m := ready(t)
	_ = m.View()
	layout := m.views.groups
	rows := m.filtered()
	next, _ := m.Update(key('j'))
	m = next.(Model)
	_ = m.View()
	if m.views.groups != layout || !sameRows(rows, m.filtered()) {
		t.Fatal("navigation rebuilt unchanged list")
	}
	next, prepare := m.Update(refreshMsg{view: m.active, result: model.Result{Complete: true, PRs: []model.PullRequest{{URL: "new", Title: "New"}}}})
	next, _ = next.(Model).Update(prepare())
	m = next.(Model)
	_ = m.View()
	if m.views.groups == layout || len(m.filtered()) != 1 || m.filtered()[0].URL != "new" {
		t.Fatal("refresh reused stale list")
	}
}

func BenchmarkGitHubViewportNavigation(b *testing.B) {
	m := readyBenchmark(b)
	snapshot := m.prs.Export()
	for i := 0; i < 5000; i++ {
		snapshot.Datasets[model.Open].PRs = append(snapshot.Datasets[model.Open].PRs, model.PullRequest{URL: fmt.Sprintf("https://github.com/acme/api/pull/%d", i+1), Repository: "acme/api", Number: i + 1, Title: fmt.Sprintf("APP-%d work", i)})
	}
	m.prs.Restore(snapshot)
	_ = m.View()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		next, _ := m.Update(key('j'))
		m = next.(Model)
		_ = m.View()
	}
}

func BenchmarkGitHubAIViewportNavigation(b *testing.B) {
	for _, n := range []int{1000, 5000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			m := readyBenchmark(b)
			m.width, m.height = 160, 40
			m.copilot = &copilotState{entities: m.entities, snoozes: m.snoozes}
			snapshot := m.prs.Export()
			for i := 0; i < n; i++ {
				pr := model.PullRequest{URL: fmt.Sprintf("https://github.com/acme/api/pull/%d", i+1), Repository: "acme/api", Number: i + 1, Title: fmt.Sprintf("APP-%d work", i/20), HeadRef: fmt.Sprintf("branch-%d", i)}
				snapshot.Datasets[model.Open].PRs = append(snapshot.Datasets[model.Open].PRs, pr)
				m.copilot.sessions = append(m.copilot.sessions, model.CopilotSession{ID: fmt.Sprint(i), Name: "session", Repository: pr.Repository, Branch: pr.HeadRef, Status: "active", LastActivity: "2026-10-27T10:00:00Z"})
			}
			m.prs.Restore(snapshot)
			_ = m.View()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				next, _ := m.Update(key(rune("jk"[i%2])))
				m = next.(Model)
				_ = m.View()
			}
		})
	}
}

func TestGitHubAIColumnFollowsRefreshAfterNavigation(t *testing.T) {
	m := ready(t)
	m.width = 160
	m.editPRs(func(snapshot *model.Snapshot) { snapshot.Datasets[model.Open].PRs[0].HeadRef = "feature" })
	m.copilot = &copilotState{entities: m.entities, snoozes: m.snoozes}
	session := model.CopilotSession{ID: "one", Repository: "acme/api", Branch: "feature", Status: "active"}
	m.copilot.sessions = []model.CopilotSession{session}
	if view := m.View().Content; !strings.Contains(view, "🟢") {
		t.Fatal("initial AI status missing", view)
	}
	for _, msg := range []tea.Msg{key('j'), key('k'), refreshTick{}, snoozeTick{}} {
		next, _ := m.Update(msg)
		m = next.(Model)
		if view := m.View().Content; !strings.Contains(view, "🟢") {
			t.Fatal("navigation or timer lost AI status", view)
		}
	}
	for _, tc := range []struct{ status, branch, icon string }{
		{"waiting", "feature", "🟡"},
		{"failed", "unrelated", ""},
		{"completed", "feature", "✅"},
	} {
		session.Status, session.Branch = tc.status, tc.branch
		next, _ := m.Update(copilotResult{sessions: []model.CopilotSession{session}})
		m = next.(Model)
		view := m.View().Content
		for _, icon := range []string{"🟢", "🟡", "🔴", "✅"} {
			if strings.Contains(view, icon) != (icon == tc.icon) {
				t.Fatalf("%s/%s: stale or missing %s in %s", tc.status, tc.branch, icon, view)
			}
		}
	}
	next, _ := m.Update(copilotResult{})
	m = next.(Model)
	if view := m.View().Content; strings.Contains(view, "✅") {
		t.Fatal("removed session retained its status", view)
	}
}

func TestGitHubAIColumnFollowsPRAndLinkChanges(t *testing.T) {
	m := ready(t)
	m.width = 160
	m.copilot = &copilotState{entities: m.entities, snoozes: m.snoozes, sessions: []model.CopilotSession{
		{ID: "one", Repository: "acme/api", Branch: "feature", Status: "active"},
	}}
	if view := m.View().Content; strings.Contains(view, "🟢") {
		t.Fatal("unrelated session shown", view)
	}
	rows := append([]model.PullRequest(nil), m.prs.Snapshot().Datasets[model.Open].PRs...)
	rows[0].HeadRef = "feature"
	next, _ := m.Update(refreshMsg{view: model.Open, prepared: true, baseRows: m.prs.Snapshot().Datasets[model.Open].PRs, result: model.Result{Complete: true, PRs: rows}})
	m = next.(Model)
	if view := m.View().Content; strings.Count(view, "🟢") != 1 {
		t.Fatal("PR refresh did not update AI relation", view)
	}
	next, _ = m.linkReferences([]model.TaskReference{prRef(rows[1])}, []model.TaskReference{copilotRef(m.copilot.sessions[0])})
	next, write := next.(Model).Update(struct{}{})
	if write == nil {
		t.Fatal("link was not queued")
	}
	// The old cache is still usable until the write completion installs the link.
	_ = next.(Model).View()
	next, _ = next.(Model).Update(write())
	m = next.(Model)
	m.sequenceNotice = ""
	if view := m.View().Content; strings.Count(view, "🟢") != 2 {
		t.Fatal("saved link did not update AI relation", view)
	}
}

func BenchmarkDetailViewportNavigation(b *testing.B) {
	m := readyBenchmark(b)
	m.detail = &detailState{data: model.PRDetails{Comments: []model.PRComment{{Body: strings.Repeat("large comment ", 10000)}}}}
	_ = m.View()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		m = next.(Model)
		_ = m.View()
	}
}

func readyBenchmark(b *testing.B) Model {
	b.Setenv("CUSTOM_VISNING_LINKS", filepath.Join(b.TempDir(), "links.sqlite"))
	b.Setenv("CUSTOM_VISNING_TASKS", filepath.Join(b.TempDir(), "tasks.sqlite"))
	b.Setenv("CUSTOM_VISNING_SNOOZE", filepath.Join(b.TempDir(), "snoozes.sqlite"))
	m := New(b.Context(), nil, nil)
	b.Cleanup(m.cancel)
	m.booting = false
	m.editPRs(func(snapshot *model.Snapshot) { snapshot.User = "alice" })
	return m
}

func TestLateJiraCacheCannotReplaceLiveOrNewerQuery(t *testing.T) {
	m := jiraModel(t)
	m.jira.generation, m.jira.query, m.jira.loading = 2, "current", true
	m.jira.rows = []jira.Issue{{Key: "KEEP"}}
	next, _ := m.Update(jiraCachedResult{generation: 1, query: "old", cached: jira.Cached{Issues: []jira.Issue{{Key: "OLD"}}}})
	m = next.(Model)
	if m.jira.rows[0].Key != "KEEP" {
		t.Fatal("old query cache replaced current query")
	}
	m.jira.loading = false
	next, _ = m.Update(jiraCachedResult{generation: 2, query: "current", cached: jira.Cached{Issues: []jira.Issue{{Key: "CACHED"}}}})
	m = next.(Model)
	if m.jira.rows[0].Key != "KEEP" {
		t.Fatal("late cache replaced live result")
	}
}

func TestMattermostResultInstallsPreparedIndexAndRetainsOrder(t *testing.T) {
	m := readerModel(t)
	result := indexedMMResult(mmResult{data: m.mmReader.data, authenticated: true})
	next, _ := m.Update(result)
	m = next.(Model)
	if !m.mmReader.indexed || len(m.mmReader.byChannel["c"]) != 1 || m.mmReader.latest["d"] != 200 {
		t.Fatal("prepared index not installed")
	}
	posts := m.mmReader.posts()
	if len(posts) != 1 || posts[0].ID != "p" {
		t.Fatal("wrong channel posts", posts)
	}
	if &posts[0] != &m.mmReader.posts()[0] {
		t.Fatal("unchanged filter rebuilt posts")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m = next.(Model)
	posts = m.mmReader.posts()
	if len(posts) != 1 || posts[0].ID != "reply" {
		t.Fatal("channel switch retained cached posts", posts)
	}
}

func TestPartialRefreshRebasesOnNewerLocalRows(t *testing.T) {
	m := ready(t)
	previous := m.prs.Snapshot().Datasets[m.active].PRs
	fresh := []model.PullRequest{{URL: "new", Title: "New partial result"}}
	result := refreshMsg{view: m.active, prepared: true, baseRows: previous, freshRows: fresh, result: model.Result{PRs: model.Merge(previous, fresh)}}
	current := append([]model.PullRequest(nil), previous...)
	current[0].Title = "Newer local detail"
	m.editPRs(func(snapshot *model.Snapshot) { snapshot.Datasets[m.active].PRs = current })
	next, prepare := m.Update(result)
	next, _ = next.(Model).Update(prepare())
	m = next.(Model)
	for _, p := range m.prs.Snapshot().Datasets[m.active].PRs {
		if p.URL == current[0].URL && p.Title != current[0].Title {
			t.Fatal("partial refresh restored an old cached row")
		}
	}
}

func TestSettingsReadFailureCanRetryWithoutEmptyValuesPanic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("CUSTOM_VISNING_CONFIG", path)
	if err := os.WriteFile(path, []byte(`{"jira":`), 0600); err != nil {
		t.Fatal(err)
	}
	m := ready(t)
	next, _ := m.openSettings()
	next, load := next.(Model).Update(struct{}{})
	next, _ = next.(Model).Update(load())
	m = next.(Model)
	if m.settings.busy || len(m.settings.values) != 0 || m.settings.notice == "" {
		t.Fatal("read failure was not reported")
	}
	next, retry := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if !m.settings.busy || retry == nil || m.settings.editing {
		t.Fatal("empty failed settings load did not retry")
	}
}

func TestCopilotDetailScrollingUsesCachedLines(t *testing.T) {
	m := ready(t).WithCopilot(testCopilot(t), true)
	m.copilot.detail = &copilot.Detail{Session: copilot.Session{ID: "one"}, Events: []copilot.Event{{Content: strings.Repeat("event content ", 1000)}}}
	_ = m.View()
	lines := m.copilot.formattedLines
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(Model)
	_ = m.View()
	if m.copilot.offset != 1 || &lines[0] != &m.copilot.formattedLines[0] {
		t.Fatal("detail scrolling changed selection or rebuilt the history")
	}
	m.setPageFlag(pageCopilot, false)
	m.copilot.loading = false
	result := m.refreshCopilot()().(copilotResult)
	if result.id != "" || result.detail != nil {
		t.Fatal("hidden details were reloaded")
	}
}

func TestUnreadableTaskStoreCannotBeOverwrittenByLocalActions(t *testing.T) {
	for _, action := range []string{"archive", "delete", "link"} {
		t.Run(action, func(t *testing.T) {
			m := ready(t)
			m.tasks.loadFailed = true
			task := model.TaskReference{Provider: "task", URL: "task://unreadable", Key: "unreadable"}
			var next tea.Model
			switch action {
			case "archive":
				next, _ = m.archiveReferences([]model.TaskReference{task})
			case "delete":
				next, _ = m.deleteReferences([]model.TaskReference{task})
			case "link":
				next, _ = m.linkReferences([]model.TaskReference{task}, []model.TaskReference{prRef(m.filtered()[0])})
			}
			m = next.(Model)
			if m.localIO.busy() || m.sequenceNotice == "" {
				t.Fatal("unreadable task history could be overwritten")
			}
		})
	}
}
