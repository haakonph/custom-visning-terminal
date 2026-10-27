package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/haakonph/custom-visning-terminal/internal/jira"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func taskKey(m Model, key string) Model {
	next, _ := update(m, tea.KeyPressMsg{Code: []rune(key)[0], Text: key})
	return next
}

func TestTasksCollectAcrossProvidersAndReload(t *testing.T) {
	t.Setenv("CUSTOM_VISNING_LINKS", filepath.Join(t.TempDir(), "links.sqlite"))
	t.Setenv("CUSTOM_VISNING_TASKS", filepath.Join(t.TempDir(), "tasks.json"))
	m := New(context.Background(), nil, nil)
	defer m.cancel()
	m.active = model.Reviews
	m.editPRs(func(snapshot *model.Snapshot) {
		snapshot.Datasets[model.Reviews].PRs = []model.PullRequest{{URL: "https://github.com/org/repo/pull/1", Repository: "org/repo", Number: 1, Title: "Review this"}}
	})
	m, _ = sequence(m, "an")
	if !(m.page == pageTasks) || len(m.tasks.items) != 1 || m.tasks.items[0].References[0].Role != "review" {
		t.Fatal("review PR should create a task")
	}
	m.setPageFlag(pageTasks, false)
	m, _ = sequence(m, "to")
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(m.tasks.items[0].References) != 1 {
		t.Fatal("adding the same PR must not duplicate it")
	}
	m.setPageFlag(pageTasks, false)
	m.jira = &jiraState{rows: []jira.Issue{{Key: "ABC-1", URL: "https://jira.example/browse/ABC-1", Title: "First issue"}, {Key: "ABC-2", URL: "https://jira.example/browse/ABC-2", Title: "Second issue"}}}
	m.setPageFlag(pageJira, true)
	m, _ = sequence(m, "to")
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m.setPageFlag(pageTasks, false)
	m.jira.selected = 1
	m, _ = sequence(m, "to")
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(m.tasks.items[0].References) != 3 {
		t.Fatal("task should contain one PR and two Jira issues")
	}
	reloaded := newTasks()
	if reloaded.loadFailed || len(reloaded.items) != 1 || len(reloaded.items[0].References) != 3 {
		t.Fatal("references should survive restart")
	}
	m.setPageFlag(pageTasks, true)
	m.tasks.details = true
	m = taskKey(taskKey(m, "a"), "d")
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
	m, _ = sequence(m, "vo")
	m, _ = sequence(m, "an")
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "broken" || len(m.tasks.items) != 0 {
		t.Fatal("corrupt file must be preserved")
	}
}

func TestTasksTitleEditingDoesNotTriggerGlobalShortcuts(t *testing.T) {
	t.Setenv("CUSTOM_VISNING_TASKS", filepath.Join(t.TempDir(), "tasks.json"))
	m := New(context.Background(), nil, nil)
	defer m.cancel()
	m, _ = sequence(m, "vo")
	m, _ = sequence(m, "an")
	m.tasks.input.SetValue("Jira og PR")
	m = taskKey(m, "g")
	if !(m.page == pageTasks) || !m.tasks.editing {
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
	t.Setenv("CUSTOM_VISNING_LINKS", filepath.Join(t.TempDir(), "links.sqlite"))
	m := New(context.Background(), &fakeSource{}, nil)
	defer m.cancel()
	m.editPRs(func(snapshot *model.Snapshot) {
		snapshot.Datasets[model.Open].PRs = []model.PullRequest{{URL: "https://github.com/org/repo/pull/1", Title: "Old title"}}
	})
	m, _ = sequence(m, "an")
	m, _ = update(m, refreshMsg{view: model.Open, result: model.Result{Complete: true, PRs: []model.PullRequest{{URL: "https://github.com/org/repo/pull/1", Title: "New title", Review: model.Approved}}}})
	if !(m.page == pageTasks) || !strings.Contains(m.tasksView().Content, "New title") {
		t.Fatal("background refresh should update task references without leaving the page")
	}
	m, _ = sequence(m, "vg")
	if m.page == pageTasks {
		t.Fatal("g should return to GitHub")
	}
	m, _ = sequence(m, "to")
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
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
	m.setPageFlag(pageTasks, true)
	m.tasks.details = false
	m, _ = sequence(m, "ad")
	if len(m.tasks.items) != 0 {
		t.Fatal("D should delete the local task")
	}
	m = taskKey(m, "j")
	m = taskKey(m, "d")
}

func TestTaskListWithFifteenTasks(t *testing.T) {
	t.Setenv("CUSTOM_VISNING_TASKS", filepath.Join(t.TempDir(), "tasks.json"))
	m := New(context.Background(), nil, nil)
	defer m.cancel()
	m.setPageFlag(pageTasks, true)
	for i := 1; i <= 15; i++ {
		m.tasks.items = append(m.tasks.items, model.Task{Title: fmt.Sprintf("Oppgave %02d", i), References: []model.TaskReference{{Provider: "github", URL: fmt.Sprintf("https://github.com/org/repo/pull/%d", i), Key: fmt.Sprintf("repo#%d", i), Title: "Endring"}}})
	}
	m.width, m.height = 112, 10
	for i := 0; i < 14; i++ {
		m = taskKey(m, "j")
	}
	if m.tasks.selected != 14 || m.tasks.row != 0 {
		t.Fatal("j should navigate the task list")
	}
	if !strings.Contains(m.View().Content, "Oppgave 15") {
		t.Fatal("selected task must remain visible after scrolling")
	}
	m, _ = sequence(m, "d")
	if !m.tasks.details {
		t.Fatal("Enter must focus details")
	}
	opened := ""
	m.open = func(_ context.Context, link string) error { opened = link; return nil }
	_, cmd := sequence(m, "en")
	if cmd == nil {
		t.Fatal("Enter in details must open reference")
	}
	cmd()
	if opened != "https://github.com/org/repo/pull/15" {
		t.Fatal("opened wrong task reference")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.tasks.details || !(m.page == pageTasks) {
		t.Fatal("Esc must return to the list")
	}
	m = taskKey(m, "/")
	m.tasks.input.SetValue("repo#8")
	m = taskKey(m, " ")
	if m.tasks.selected != 7 || len(m.tasks.visibleTasks()) != 1 {
		t.Fatal("search should match references and select visible task")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = sequence(m, "an")
	if m.tasks.query != "" || m.tasks.selected != 15 {
		t.Fatal("new task must be visible even after filtering")
	}
}

func TestTaskPanelsResizeAndEmptySearch(t *testing.T) {
	t.Setenv("CUSTOM_VISNING_TASKS", filepath.Join(t.TempDir(), "tasks.json"))
	m := New(context.Background(), nil, nil)
	defer m.cancel()
	m.setPageFlag(pageTasks, true)
	m.tasks.items = []model.Task{{Title: "Oppgaven min", References: []model.TaskReference{{Title: "Referansen min"}}}}
	for _, size := range [][2]int{{112, 24}, {90, 10}, {89, 10}, {40, 8}, {10, 2}} {
		for _, details := range []bool{false, true} {
			m.tasks.details = details
			m, _ = update(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			content := m.View().Content
			lines := strings.Split(content, "\n")
			if len(lines) > size[1] {
				t.Fatal("view exceeds height")
			}
			for _, line := range lines {
				if ansi.StringWidth(line) > size[0] {
					t.Fatal("view exceeds width")
				}
			}
			if size[0] >= 90 && (!strings.Contains(content, "Oppgaven min") || !strings.Contains(content, "Referansen min")) {
				t.Fatal("wide view should show both panels")
			}
			if size[0] == 40 && !details && strings.Contains(content, "Referansen min") {
				t.Fatal("narrow list should show tasks only")
			}
		}
	}
	m.tasks.query = "no match"
	for _, key := range []string{"d", "D", "e"} {
		m = taskKey(m, key)
	}
	if len(m.tasks.items) != 1 || len(m.tasks.items[0].References) != 1 || m.tasks.editing {
		t.Fatal("empty search must not act on hidden task")
	}
}

func TestLegacyTasksMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	t.Setenv("CUSTOM_VISNING_TASKS", path)
	original := `[{"title":"Existing task","references":[{"provider":"jira","key":"ABC-1","url":"https://jira.example/browse/ABC-1"}]}]`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	tasks := newTasks()
	if tasks.loadFailed || len(tasks.items) != 1 || len(tasks.items[0].References) != 1 {
		t.Fatal(tasks)
	}
	tasks.items[0].Title = "Updated"
	tasks.save()
	if b, _ := os.ReadFile(path); string(b) != original {
		t.Fatal("legacy tasks changed")
	}
	reloaded := newTasks()
	if reloaded.loadFailed || reloaded.items[0].Title != "Updated" {
		t.Fatal("database not used after migration")
	}
}
