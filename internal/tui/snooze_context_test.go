package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/snooze"
)

func TestSnoozeImplicitOne(t *testing.T) {
	for _, unit := range []string{"h", "d", "m", "D"} {
		t.Run(unit, func(t *testing.T) {
			m := snoozeReady(t)
			now := m.snoozes.at
			m.snoozes.now = func() time.Time { return now }
			id := snooze.Identity("github", m.filtered()[0].URL)
			m = press(m, "z"+unit)
			deadlineUnit := unit
			if unit == "m" {
				deadlineUnit = "D"
			}
			want, err := snooze.Deadline(now.In(time.Local), "1", deadlineUnit)
			if err != nil {
				t.Fatal(err)
			}
			if m.snoozes.input || !m.snoozes.entries[id].Until.Equal(want) {
				t.Fatal("missing count did not mean one")
			}
		})
	}
}

func contextModel(t *testing.T) Model {
	t.Helper()
	m := unifiedModel(t)
	m.snoozes.store.Path = t.TempDir() + "/snooze.sqlite"
	// A second task shares a PR but must remain active when the first is paused.
	m.tasks.items = append(m.tasks.items, model.Task{ID: "shared", Title: "Other work", References: []model.TaskReference{m.tasks.items[0].References[0]}})
	if err := m.tasks.save(); err != nil {
		t.Fatal(err)
	}
	m.tasks.active = true
	m.tasks.selected = 0
	return m
}

func TestContextSnoozePreviewCancelAndSharedTask(t *testing.T) {
	m := contextModel(t)
	m = press(m, "zz")
	if !m.snoozes.input || !m.snoozes.whole || len(m.snoozes.entries) != 0 {
		t.Fatal("zz saved before choosing a time")
	}
	if view := m.View().Content; !strings.Contains(view, "hele sammenhengen") || !strings.Contains(view, "Jira-saker") || !strings.Contains(view, "PR-er") {
		t.Fatal("scope is not visible", view)
	}
	for _, r := range m.snoozes.target.Members {
		if r.Provider == "task" && r.Key == "shared" {
			t.Fatal("shared PR pulled in another task")
		}
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.snoozes.input || len(m.snoozes.entries) != 0 || len(m.tasks.visibleTasks()) != 2 {
		t.Fatal("cancel changed snoozes")
	}
}

func TestContextSnoozePersistsReschedulesAndRestoresAsGroup(t *testing.T) {
	m := contextModel(t)
	now := m.snoozes.at
	m.snoozes.now = func() time.Time { return now }
	pr := m.tasks.items[0].References[0]
	separate := snooze.Entry{ID: refSnoozeID(pr), Kind: pr.Provider, URL: pr.URL, Title: pr.Title, Until: now.Add(4 * time.Hour)}
	if err := m.snoozes.commit(separate, false); err != nil {
		t.Fatal(err)
	}
	m = press(m, "zzh")
	groupID := "context:task:task1"
	group, ok := m.snoozes.entries[groupID]
	if !ok || !group.Until.Equal(now.Add(time.Hour)) || len(group.Roots) != 1 || len(group.Members) < 4 {
		t.Fatal("missing context group", group)
	}
	if !m.snoozes.hidden("task:task1") || m.snoozes.hidden("task:shared") || len(m.tasks.visibleTasks()) != 1 {
		t.Fatal("wrong task visibility")
	}
	for _, r := range group.Members {
		if !m.snoozes.hidden(refSnoozeID(r)) {
			t.Fatal("member visible", r)
		}
	}
	loaded, err := m.snoozes.store.Load()
	if err != nil || len(loaded[groupID].Members) != len(group.Members) || len(loaded[groupID].Roots) != 1 {
		t.Fatal("group lost on reload", err)
	}
	// New() reloads the same group without provider data or a network request.
	t.Setenv("CUSTOM_VISNING_SNOOZE", m.snoozes.store.Path)
	restarted := New(context.Background(), nil, nil)
	t.Cleanup(restarted.cancel)
	if !restarted.snoozes.hidden("task:task1") || len(restarted.snoozes.entries[groupID].Members) != len(group.Members) {
		t.Fatal("restart lost group")
	}
	m.snoozes.active = true
	for i, e := range m.snoozes.rows() {
		if e.ID == groupID {
			m.snoozes.selected = i
		}
	}
	m = press(m, "z2h")
	if e := m.snoozes.entries[groupID]; !e.Until.Equal(now.Add(2*time.Hour)) || len(e.Members) != len(group.Members) {
		t.Fatal("reschedule dropped members")
	}
	for i, e := range m.snoozes.rows() {
		if e.ID == groupID {
			m.snoozes.selected = i
		}
	}
	m, _ = sequence(m, "az")
	if _, ok := m.snoozes.entries[groupID]; ok {
		t.Fatal("restore did not remove group")
	}
	if m.snoozes.hidden("task:task1") || !m.snoozes.hidden(separate.ID) || len(m.snoozes.entries) != 1 {
		t.Fatal("restore changed separate snooze")
	}
}

func TestContextSnoozeNewRelationsWriteFailureAndExpiry(t *testing.T) {
	m := contextModel(t)
	now := m.snoozes.at
	m.snoozes.now = func() time.Time { return now }
	m = press(m, "zzh")
	newPR := model.PullRequest{URL: "https://github.com/acme/api/pull/999", Repository: "acme/api", Number: 999, Title: "APP-1 Follow-up"}
	m.snapshot.Datasets[0].PRs = append(m.snapshot.Datasets[0].PRs, newPR)
	id := snooze.Identity("github", newPR.URL)
	path := m.snoozes.store.Path
	m.snoozes.store.Path = t.TempDir()
	m, _ = update(m, snoozeTick{})
	if m.snoozes.hidden(id) {
		t.Fatal("failed write hid new member")
	}
	m.snoozes.store.Path = path
	m, _ = update(m, snoozeTick{})
	if !m.snoozes.hidden(id) {
		t.Fatal("new related PR escaped context snooze")
	}
	loaded, err := m.snoozes.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range loaded["context:task:task1"].Members {
		if refSnoozeID(r) == id {
			found = true
		}
	}
	if !found {
		t.Fatal("new member not persisted")
	}
	now = now.Add(time.Hour)
	m, _ = update(m, snoozeTick{})
	if m.snoozes.hidden(id) || m.snoozes.hidden("task:task1") || len(m.snoozes.rows()) != 0 {
		t.Fatal("group did not expire together")
	}
}

func TestContextSnoozeWriteFailureLeavesAllVisible(t *testing.T) {
	m := contextModel(t)
	m.snoozes.store.Path = t.TempDir()
	m = press(m, "zzh")
	if !m.snoozes.input || len(m.snoozes.entries) != 0 || len(m.tasks.visibleTasks()) != 2 {
		t.Fatal("failed group save changed visibility")
	}
}

func TestContextSnoozeFromEveryProvider(t *testing.T) {
	for _, page := range []string{"g", "t", "a", "c", "o"} {
		t.Run(page, func(t *testing.T) {
			m := contextModel(t)
			m, _ = sequence(m, "v"+page)
			roots := m.currentReferences()
			if len(roots) == 0 {
				t.Fatal("missing fixture selection")
			}
			m = press(m, "zzh")
			if m.snoozes.input || len(m.snoozes.entries) != 1 {
				t.Fatal("zzh not handled by provider", page)
			}
			for _, r := range roots {
				if !m.snoozes.hidden(refSnoozeID(r)) {
					t.Fatal("root escaped group", r)
				}
			}
		})
	}
}

func TestOverlappingContextSnoozesRemainIndependent(t *testing.T) {
	m := contextModel(t)
	now := m.snoozes.at
	m.snoozes.now = func() time.Time { return now }
	sharedPR := m.tasks.items[0].References[0]
	m = press(m, "zzh")
	// The remaining task shares the PR. Snooze that task as a separate group.
	m = press(m, "zz2h")
	if len(m.snoozes.rows()) != 2 {
		t.Fatal("groups collapsed")
	}
	m.snoozes.active = true
	for i, e := range m.snoozes.rows() {
		if e.ID == "context:task:task1" {
			m.snoozes.selected = i
		}
	}
	m, _ = sequence(m, "az")
	if !m.snoozes.hidden(refSnoozeID(sharedPR)) || m.snoozes.hidden("task:task1") || !m.snoozes.hidden("task:shared") {
		t.Fatal("restoring a group changed an overlapping group")
	}
	for i, e := range m.snoozes.rows() {
		if e.ID == "context:task:shared" {
			m.snoozes.selected = i
		}
	}
	m, _ = sequence(m, "az")
	if m.snoozes.hidden(refSnoozeID(sharedPR)) || len(m.snoozes.rows()) != 0 {
		t.Fatal("last group did not restore member")
	}
}

func TestContextSnoozeMultipleRootsAndDetails(t *testing.T) {
	m := contextModel(t)
	m, _ = sequence(m, " j ") // select both tasks with Space, j, Space
	m = press(m, "zz2h")
	if len(m.snoozes.rows()) != 1 {
		t.Fatal("selection did not produce one group")
	}
	entry := m.snoozes.rows()[0]
	if len(entry.Roots) != 2 || !m.snoozes.hidden("task:task1") || !m.snoozes.hidden("task:shared") {
		t.Fatal("group omitted selected root")
	}
	m, _ = sequence(m, "vzd")
	if view := m.View().Content; !strings.Contains(view, "Hele sammenhengen") || !strings.Contains(view, "Other work") {
		t.Fatal("group details omitted members", view)
	}
}
