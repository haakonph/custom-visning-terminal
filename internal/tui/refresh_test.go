package tui

import (
	"errors"
	"testing"
	"time"

	"github.com/haakonph/custom-visning-terminal/internal/connectors/jira"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestAutoRefreshPriorityCompletionAndBackoff(t *testing.T) {
	m := ready(t)
	m.auto.options = defaultRefreshOptions()
	now := time.Now()
	for v := model.Open; v < model.ViewCount; v++ {
		m.auto.github[v].finished = now
	}
	m.auto.github[m.active].finished = now.Add(-time.Minute)
	m, _ = update(m, refreshTick(now))
	if !m.loading[m.active] {
		t.Fatal("active list not scheduled")
	}
	m, _ = update(m, refreshTick(now.Add(time.Second)))
	for v := model.Open; v < model.ViewCount; v++ {
		if v != m.active && m.loading[v] {
			t.Fatal("overlapping list scheduled")
		}
	}
	m, _ = update(m, refreshMsg{view: m.active, err: errors.New("offline")})
	if m.auto.github[m.active].failures != 1 {
		t.Fatal("completion was not retained on model")
	}
	m, _ = update(m, refreshTick(now.Add(time.Minute)))
	if m.loading[m.active] {
		t.Fatal("failure did not back off")
	}
	m.auto.options.Enabled = false
	m.auto.github[m.active] = refreshJob{}
	m, _ = update(m, refreshTick(now.Add(time.Hour)))
	if m.loading[m.active] {
		t.Fatal("disabled scheduler fetched data")
	}
	// Manual refresh bypasses scheduler backoff and disabled state.
	if m.refresh(m.active) == nil {
		t.Fatal("manual refresh disabled")
	}
}

func TestJiraAutoRefreshDoesNotCancelSameQueryAndKeepsSelection(t *testing.T) {
	m := jiraModel(t)
	defer m.cancel()
	_ = m.refreshJira()
	generation := m.jira.generation
	if m.refreshJira() != nil || m.jira.generation != generation {
		t.Fatal("duplicate request restarted")
	}
	m.jira.rows = []jira.Issue{{Key: "APP-1"}, {Key: "APP-2"}}
	m.jira.selected = 1
	m, _ = update(m, jiraResult{generation: generation, query: m.jira.query, rows: []jira.Issue{{Key: "APP-2"}, {Key: "APP-1"}}, at: time.Now()})
	if m.jira.selected != 0 || m.jira.filtered()[0].Key != "APP-2" {
		t.Fatal("selection moved to another issue")
	}
	// Jira-only startup must not depend on GitHub bootstrap finishing.
	m.booting = true
	m.auto.jira[m.jira.query].finished = time.Now().Add(-2 * time.Minute)
	m.auto.jira[m.jira.query].started = time.Time{}
	m, _ = update(m, refreshTick(time.Now()))
	if !m.jira.loading {
		t.Fatal("Jira-only automatic refresh not scheduled")
	}
}

func TestJiraBackgroundResultCannotOverwriteNewerForeground(t *testing.T) {
	m := jiraModel(t)
	defer m.cancel()
	_ = m.refreshJira()
	query := m.jira.query
	old := time.Now().Add(-time.Minute)
	m.auto.jira[query].started = time.Now()
	m, _ = update(m, jiraBackgroundResult{started: old, query: query, err: errors.New("old failure"), at: time.Now()})
	if m.auto.jira[query].failures != 0 {
		t.Fatal("stale background response changed polling state")
	}
}

func TestDetailsFailureRetainsDataAndOffset(t *testing.T) {
	m := ready(t)
	m.detail = &detailState{request: 1, loading: true, offset: 12, data: model.PRDetails{Files: []model.ChangedFile{{Path: "keep.go"}}}}
	m, _ = update(m, detailMsg{request: 1, err: errors.New("offline")})
	if m.detail.loading || len(m.detail.data.Files) != 1 || m.detail.offset != 12 {
		t.Fatal("failed refresh lost detail state")
	}
}

func TestRefreshDuringSettingsRetainsEditorAndCompletesDetails(t *testing.T) {
	m := ready(t)
	m.settings.active = true
	m.settings.editing = true
	m.settings.input.SetValue("pending edit")
	m.detail = &detailState{request: 9, loading: true, offset: 7}
	m, _ = update(m, detailMsg{request: 9, data: model.PRDetails{Files: []model.ChangedFile{{Path: "a.go"}}}})
	if m.detail.loading || len(m.detail.data.Files) != 1 || m.settings.input.Value() != "pending edit" || !m.settings.editing {
		t.Fatal("settings swallowed completion or lost editing state")
	}
}
func TestBackgroundGitHubWaitsForBackgroundInterval(t *testing.T) {
	m := ready(t)
	now := time.Now()
	for v := model.Open; v < model.ViewCount; v++ {
		m.auto.github[v].finished = now.Add(-2 * time.Minute)
	}
	m.tasks.active = true
	m, _ = update(m, refreshTick(now))
	for _, loading := range m.loading {
		if loading {
			t.Fatal("background view fetched at active interval")
		}
	}
	m, _ = update(m, refreshTick(now.Add(4*time.Minute)))
	count := 0
	for _, loading := range m.loading {
		if loading {
			count++
		}
	}
	if count != 1 {
		t.Fatal("expected one background fetch", count)
	}
}

func TestJiraInitialQueryIsTrackedAcrossInitCopy(t *testing.T) {
	m := jiraModel(t)
	defer m.cancel()
	_ = m.Init()
	if m.auto.jira[m.jira.query] == nil {
		t.Fatal("initial Jira query was lost by value receiver")
	}
}

func TestRefreshSettingsApplyWithoutRestart(t *testing.T) {
	m := jiraModel(t)
	defer m.cancel()
	opened, _ := m.openSettings()
	m = opened.(Model)
	index := -1
	for i, field := range settingFields {
		if field.path == "refresh.active" {
			index = i
			break
		}
	}
	if index < 0 {
		t.Fatal("refresh setting missing")
	}
	m = editSetting(t, m, index, "10s")
	if m.settings.editing || m.auto.options.Active != "10s" {
		t.Fatal("setting not applied", m.settings.notice)
	}
	m = editSetting(t, m, index, "1ms")
	if !m.settings.editing || m.auto.options.Active != "10s" {
		t.Fatal("invalid polling interval accepted")
	}
}
