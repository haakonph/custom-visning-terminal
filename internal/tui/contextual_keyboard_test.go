package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestActionMenusAndHelpBelongToVisiblePage(t *testing.T) {
	for _, tc := range []struct {
		page, present, absent, blocked string
	}{
		{"g", "be om Copilot-review", "flytt valgte saker", "af"},
		{"t", "flytt valgte saker", "automerge på", "a+"},
		{"a", "åpne worktree i IntelliJ", "be om Copilot-review", "ar"},
		{"c", "merk kanal lest", "endre oppgavetittel", "ae"},
		{"o", "endre oppgavetittel", "tilordne meg", "am"},
		{"z", "hent tilbake", "arkiver valget lokalt", "ak"},
	} {
		t.Run(tc.page, func(t *testing.T) {
			m := unifiedModel(t)
			m, _ = sequence(m, "v"+tc.page)
			m.width, m.height = 200, 200
			m, _ = sequence(m, "?")
			help := m.View().Content
			if !strings.Contains(help, tc.present) || strings.Contains(help, tc.absent) {
				t.Fatalf("help offered actions from another page: %s", help)
			}
			m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
			m, _ = sequence(m, tc.blocked[:1])
			menu := m.View().Content
			if strings.Contains(menu, tc.absent) {
				t.Fatalf("menu offered an unrelated action: %s", menu)
			}
			m, cmd := sequence(m, tc.blocked[1:])
			if cmd != nil || m.sequence != "" || len(m.sequenceRefs) != 0 || m.jiraAction != nil || m.autoMergeBusy || m.reviewRequestBusy || m.tasks.editing || len(m.entities.data.Archived) != 0 {
				t.Fatal("unavailable action ran or retained a target")
			}
		})
	}
}

func TestRelatedPickerRequiresOpeningTheTargetBeforeActions(t *testing.T) {
	m := unifiedModel(t)
	next, _ := m.showEntities("Velg sak", "related", []model.TaskReference{jiraRef("APP-1", m)}, nil)
	m = next.(Model)
	m, cmd := sequence(m, "am")
	if cmd != nil || m.dialog == nil || m.jiraAction != nil || m.jiraActive {
		t.Fatal("Jira action ran from a picker above GitHub")
	}
	rows := m.jira.rows
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = update(m, jiraResult{generation: m.jira.generation, query: m.jira.query, rows: rows[:1]})
	m, cmd = sequence(m, "am")
	if cmd == nil || m.visibleProvider() != "jira" || m.jiraAction == nil || len(m.jiraAction.keys) != 1 || m.jiraAction.keys[0] != "APP-1" {
		t.Fatal("Jira action unavailable after opening Jira")
	}
}

func TestDetailsOwnActionTargetOverUnderlyingSelection(t *testing.T) {
	for _, page := range []string{"g", "t", "a"} {
		t.Run(page, func(t *testing.T) {
			t.Setenv("PATH", t.TempDir())
			m := unifiedModel(t).WithMattermost(&fakeMessageSender{})
			m, _ = sequence(m, "v"+page)
			if page == "a" {
				m, _ = update(m, copilotResult{sessions: m.copilot.sessions})
			}
			m, _ = sequence(m, " j")
			visible := m.visibleReferences()
			expected := visible[m.cursor()]
			m, _ = sequence(m, "d")
			if !m.detailsOpen() || !m.hasChosen() {
				t.Fatal("expected open details above a selected list")
			}
			m, _ = sequence(m, "hlx /")
			refs := m.currentReferences()
			if len(refs) != 1 || refID(refs[0]) != refID(expected) || !m.hasChosen() || m.filtering || m.jira.mode != "" || m.copilot.filtering {
				t.Fatal("detail keys changed a hidden list or its selection")
			}
			m, _ = sequence(m, "as")
			if !m.mattermost.active || !strings.Contains(m.mattermost.text, expected.Title) {
				t.Fatal("detail action used a hidden selection")
			}
		})
	}
}

func TestEditorCannotImplicitlyOpenRelatedProjects(t *testing.T) {
	m := unifiedModel(t)
	next, cmd := m.openReferences([]model.TaskReference{jiraRef("APP-1", m)}, true)
	if cmd != nil || next.(Model).sequenceNotice == "" || m.copilot.openingIDE {
		t.Fatal("editor followed a relation from a Jira issue")
	}
}

func TestDiagnosticsDoNotExposeSessionActions(t *testing.T) {
	m := unifiedModel(t)
	m, _ = sequence(m, "vavi")
	m, _ = sequence(m, "e")
	if m.sequence != "" || strings.Contains(m.helpText(), "åpne chatmappe") || strings.Contains(m.helpText(), "arkiver valget") {
		t.Fatal("diagnostics offered actions on a hidden session")
	}
	m, cmd := sequence(m, "z")
	if cmd != nil || m.snoozes.input {
		t.Fatal("diagnostics snoozed a hidden session")
	}
}
