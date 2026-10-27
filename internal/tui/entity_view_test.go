package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestEntityTableUsers(t *testing.T) {
	m := ready(t)
	m.jira = &jiraState{rows: []model.JiraIssue{{Key: "ABC-1", Assignee: "bob"}}}
	m.mmReader = &mattermostReader{data: model.MattermostData{Posts: []model.MattermostPost{
		{ID: "post-1", Author: "carol"}, {ID: "post-2", UserID: "user-id"},
	}}}
	for _, tc := range []struct {
		ref  model.TaskReference
		want string
	}{
		{prRef(m.snapshot.Datasets[0].PRs[0]), "alice"},
		{model.TaskReference{Provider: "jira", Key: "ABC-1"}, "bob"},
		{model.TaskReference{Provider: "mattermost", Key: "post-1"}, "carol"},
		{model.TaskReference{Provider: "mattermost", Key: "post-2"}, "user-id"},
		{model.TaskReference{Provider: "github", URL: "missing"}, "—"},
		{model.TaskReference{Provider: "task"}, "—"},
		{model.TaskReference{Provider: "copilot"}, "—"},
	} {
		t.Run(tc.ref.Provider+tc.ref.Key+tc.ref.URL, func(t *testing.T) {
			if got := m.referenceUser(tc.ref); got != tc.want {
				t.Fatalf("user = %q, want %q", got, tc.want)
			}
		})
	}
	// References without a configured provider must still render safely.
	m.jira, m.mmReader = nil, nil
	for _, provider := range []string{"jira", "mattermost"} {
		if got := m.referenceUser(model.TaskReference{Provider: provider}); got != "—" {
			t.Fatalf("unconfigured %s user = %q", provider, got)
		}
	}
}

func TestRelatedAIStatuses(t *testing.T) {
	m := ready(t)
	m.height = 12
	m.copilot = &copilotState{sessions: []model.CopilotSession{
		{ID: "ai-1", Name: "First session", Status: "working"},
		{ID: "ai-2", Name: "Second session", Status: "idle"},
	}}
	refs := []model.TaskReference{copilotRef(m.copilot.sessions[0]), copilotRef(m.copilot.sessions[1])}
	refs = append(refs, copilotRef(model.CopilotSession{ID: "unloaded", Name: "Unloaded session"}))
	next, _ := m.showEntities("Relatert: copilot", "related", refs, nil)
	m = next.(Model)
	for _, width := range []int{32, 50, 80, 120} {
		m.width = width
		view := ansi.Strip(m.entityView().Content)
		for _, want := range []string{"STATUS", "working", "idle"} {
			if !strings.Contains(view, want) {
				t.Fatalf("width %d: missing %q in %s", width, want, view)
			}
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("width %d: overflowing line %q", width, line)
			}
		}
	}
	m.copilot.sessions[0].Status = "finished"
	if view := ansi.Strip(m.entityView().Content); !strings.Contains(view, "finished") || strings.Contains(view, "working") {
		t.Fatalf("status did not follow loaded session: %s", view)
	}
	m.copilot = nil
	if view := ansi.Strip(m.entityView().Content); strings.Contains(view, "finished") {
		t.Fatalf("unconfigured AI retained status: %s", view)
	}
}

func TestEntityTableLayout(t *testing.T) {
	m := ready(t)
	r := prRef(m.snapshot.Datasets[0].PRs[0])
	r.Title = "修正 pålogging\n" + strings.Repeat("lang tittel ", 30)
	for _, width := range []int{1, 12, 32, 49, 50, 79, 80, 120} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m.width = width
			header := ansi.Strip(m.entityTableRow(model.TaskReference{}, true, false, false))
			row := ansi.Strip(m.entityTableRow(r, false, true, true))
			if strings.Contains(row, "\n") || ansi.StringWidth(row) > width || ansi.StringWidth(header) > width {
				t.Fatalf("table exceeds width %d: %q / %q", width, header, row)
			}
			if width >= 32 {
				userColumn := strings.Index(header, "BRUKER")
				userValue := strings.Index(row, "alice")
				if userColumn < 0 || userValue < 0 || ansi.StringWidth(header[:userColumn]) != ansi.StringWidth(row[:userValue]) || !strings.HasPrefix(row, "> ✓ ") {
					t.Fatalf("user column or selection misaligned: %q / %q", header, row)
				}
			}
		})
	}
}

func TestEntityTableSearchSelectionAndScroll(t *testing.T) {
	m := ready(t)
	m.width, m.height = 100, 6
	var refs []model.TaskReference
	for i := 0; i < 10; i++ {
		refs = append(refs, model.TaskReference{Provider: "jira", Key: fmt.Sprintf("ABC-%d", i), Title: fmt.Sprintf("Issue %d", i)})
	}
	next, _ := m.showEntities("Relatert: jira", "related", refs, nil)
	m = next.(Model)
	for i := 0; i < 9; i++ {
		m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	view := ansi.Strip(m.entityView().Content)
	if !strings.Contains(view, "BRUKER") || !strings.Contains(view, "ABC-9") || !strings.Contains(view, "> ✓ ") || len(strings.Split(view, "\n")) > m.height {
		t.Fatalf("header or selected row missing from scrolled view: %s", view)
	}
	for height := 1; height <= 5; height++ {
		m.height = height
		view = ansi.Strip(m.entityView().Content)
		if !strings.Contains(view, "ABC-9") || len(strings.Split(view, "\n")) > height {
			t.Fatalf("selected row hidden at height %d: %s", height, view)
		}
	}
	m.height = 6
	m, _ = update(m, key('/'))
	m, _ = update(m, tea.KeyPressMsg{Code: '7', Text: "7"})
	view = ansi.Strip(m.entityView().Content)
	if !strings.Contains(view, "ABC-7") || strings.Contains(view, "ABC-9") || len(strings.Split(view, "\n")) > m.height {
		t.Fatalf("search did not filter table: %s", view)
	}
	if _, ok := m.dialog.chosen[refID(refs[9])]; !ok {
		t.Fatal("search lost selection")
	}
}
