package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/local"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestEntityPickersSearchWhileTyping(t *testing.T) {
	for _, mode := range []string{"link", "related", "external"} {
		t.Run(mode, func(t *testing.T) {
			m := unifiedModel(t)
			rows := []model.TaskReference{
				{Provider: "task", URL: "task://1", Title: "Annet"},
				{Provider: "task", URL: "task://2", Title: "Jåk første"},
				{Provider: "task", URL: "task://3", Title: "Jåk andre"},
			}
			next, _ := m.showEntities("Velg", mode, rows, nil)
			m = next.(Model)
			m, _ = sequence(m, "jåk")
			if m.dialog.query != "jåk" || len(m.dialog.filtered()) != 2 || m.sequence != "" {
				t.Fatal("letters did not filter picker")
			}
			m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
			if m.dialog.selected != 1 {
				t.Fatal("arrow did not navigate filtered rows")
			}
			m, _ = update(m, tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
			m, _ = sequence(m, "xyz")
			if len(m.dialog.filtered()) != 0 || len(m.dialog.chosen) != 1 {
				t.Fatal("search lost selection or failed to show no results")
			}
			m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
			if m.dialog == nil || m.dialog.query != "" || len(m.dialog.chosen) != 1 {
				t.Fatal("Escape did not clear search and preserve selection")
			}
			m, _ = sequence(m, "jå")
			m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
			if m.dialog.query != "j" {
				t.Fatal("Backspace did not remove Unicode character")
			}
			m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyTab})
			m, _ = sequence(m, "a")
			if m.sequence != "a" || m.dialog.query != "j" {
				t.Fatal("Tab did not enable actions")
			}
		})
	}
}

func TestLinkPickerAcceptsFilteredTargetImmediately(t *testing.T) {
	m := unifiedModel(t)
	m, _ = sequence(m, "ttAPP-2")
	if m.dialog == nil || len(m.dialog.filtered()) != 1 {
		t.Fatal("target not filtered")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.dialog != nil || len(m.entities.data.Links) != 1 {
		t.Fatal("Enter did not link filtered target")
	}
	link := m.entities.data.Links[0]
	if link.Left.Key != "APP-2" && link.Right.Key != "APP-2" {
		t.Fatal("linked the wrong target")
	}
}

func TestRepositoryPickerSearchWhileTyping(t *testing.T) {
	m := ready(t)
	m.folders.picking = true
	m.folders.repositories = []string{"acme/api", "acme/jira", "acme/jira-tools"}
	m, _ = sequence(m, "acme/jira")
	if m.folders.query != "acme/jira" || len(m.folderRepositories()) != 2 || m.folders.searching || m.sequence != "" {
		t.Fatal("repository picker did not own text")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	if m.folders.selected != 1 {
		t.Fatal("repository arrow navigation failed")
	}
}

func TestChannelPickerSearchWhileTyping(t *testing.T) {
	m := readerModel(t)
	m, _ = sequence(m, "vkfront")
	if m.mmReader.input.Value() != "front" || len(m.mmReader.choices()) != 1 || m.mmReader.choices()[0].Name != "frontend" {
		t.Fatal("channel picker did not filter directly")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.mmReader.choosing {
		t.Fatal("channel picker did not close")
	}
}

func TestEditorPickerSearchAndNoResults(t *testing.T) {
	m := ready(t).WithLocal(&local.WorktreeManager{}, local.Editors{})
	m, _ = sequence(m, "evcode")
	if got := m.local.editorChoices(); len(got) != 1 || got[0] != "VS Code" {
		t.Fatal("editor not filtered")
	}
	m, _ = sequence(m, "xyz")
	m, cmd := update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || !m.local.choosing || !strings.Contains(m.View().Content, "Ingen treff") {
		t.Fatal("empty editor result was accepted")
	}
}
