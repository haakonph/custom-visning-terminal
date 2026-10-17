package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/jira"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func taskKey(m Model, key string) Model {
	next, _ := m.Update(tea.KeyPressMsg{Code: []rune(key)[0], Text: key})
	return next.(Model)
}

func TestTasksCollectAcrossProvidersAndReload(t *testing.T) {
	t.Setenv("CUSTOM_VISNING_TASKS", filepath.Join(t.TempDir(), "tasks.json"))
	m := New(context.Background(), nil, nil)
	defer m.cancel()
	m.active = model.Reviews
	m.snapshot.Datasets[model.Reviews].PRs = []model.PullRequest{{URL: "https://github.com/org/repo/pull/1", Repository: "org/repo", Number: 1, Title: "Review this"}}
	m = taskKey(m, "t")
	if !m.tasks.active || len(m.tasks.items) != 1 || m.tasks.items[0].References[0].Role != "review" {
		t.Fatal("review PR should create a task")
	}
	m = taskKey(m, "T")
	m = taskKey(m, "a")
	if len(m.tasks.items[0].References) != 1 {
		t.Fatal("adding the same PR must not duplicate it")
	}
	m.tasks.active = false
	m.jira = &jiraState{rows: []jira.Issue{{Key: "ABC-1", URL: "https://jira.example/browse/ABC-1", Title: "First issue"}, {Key: "ABC-2", URL: "https://jira.example/browse/ABC-2", Title: "Second issue"}}}
	m.jiraActive = true
	m = taskKey(m, "a")
	m.tasks.active = false
	m.jira.selected = 1
	m = taskKey(m, "a")
	if len(m.tasks.items[0].References) != 3 {
		t.Fatal("task should contain one PR and two Jira issues")
	}
	reloaded := newTasks()
	if reloaded.loadFailed || len(reloaded.items) != 1 || len(reloaded.items[0].References) != 3 {
		t.Fatal("references should survive restart")
	}
	m = taskKey(m, "d")
	if len(m.tasks.items[0].References) != 2 {
		t.Fatal("d should remove only selected reference")
	}
	if len(m.jira.rows) != 2 {
		t.Fatal("removing reference must not remove provider data")
	}
}

func TestTasksCorruptStorageIsNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	t.Setenv("CUSTOM_VISNING_TASKS", path)
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	m := New(context.Background(), nil, nil)
	defer m.cancel()
	m = taskKey(m, "T")
	m = taskKey(m, "n")
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "broken" || len(m.tasks.items) != 0 {
		t.Fatal("corrupt file must be preserved")
	}
}

func TestTasksTitleEditingDoesNotTriggerGlobalShortcuts(t *testing.T) {
	t.Setenv("CUSTOM_VISNING_TASKS", filepath.Join(t.TempDir(), "tasks.json"))
	m := New(context.Background(), nil, nil)
	defer m.cancel()
	m = taskKey(m, "T")
	m = taskKey(m, "n")
	m.tasks.input.SetValue("Jira og PR")
	m = taskKey(m, "g")
	if !m.tasks.active || !m.tasks.editing {
		t.Fatal("typing g must not switch provider")
	}
	m, _ = func() (Model, tea.Cmd) {
		next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		return next.(Model), cmd
	}()
	if m.tasks.editing || !strings.Contains(m.tasks.items[0].Title, "Jira") {
		t.Fatal("Enter should save the title")
	}
}

func TestTaskProviderNavigationAndBackgroundRefresh(t *testing.T) {
	t.Setenv("CUSTOM_VISNING_TASKS", filepath.Join(t.TempDir(), "tasks.json"))
	m := New(context.Background(), &fakeSource{}, nil)
	defer m.cancel()
	m.snapshot.Datasets[model.Open].PRs = []model.PullRequest{{URL: "https://github.com/org/repo/pull/1", Title: "Old title"}}
	m = taskKey(m, "t")
	m, _ = update(m, refreshMsg{view: model.Open, result: model.Result{Complete: true, PRs: []model.PullRequest{{URL: "https://github.com/org/repo/pull/1", Title: "New title", Review: model.Approved}}}})
	if !m.tasks.active || !strings.Contains(m.tasksView().Content, "New title") {
		t.Fatal("background refresh should update task references without leaving the page")
	}
	m = taskKey(m, "g")
	if m.tasks.active {
		t.Fatal("g should return to GitHub")
	}
	m = taskKey(m, "a")
	if len(m.tasks.items) != 1 || len(m.tasks.items[0].References) != 1 {
		t.Fatal("returning to provider must preserve target task")
	}
	for _, size := range [][2]int{{100, 24}, {40, 10}, {10, 2}} {
		m, _ = update(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		content := m.View().Content
		if len(strings.Split(content, "\n")) > size[1] {
			t.Fatal("task page exceeds terminal height")
		}
	}
	m = taskKey(m, "D")
	if len(m.tasks.items) != 0 {
		t.Fatal("D should delete the local task")
	}
	m = taskKey(m, "j")
	m = taskKey(m, "d")
}
