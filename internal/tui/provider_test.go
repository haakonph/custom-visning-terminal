package tui

import (
	"testing"

	"github.com/haakonph/custom-visning-terminal/internal/jira"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestProviderShortcutsPreserveFiltersAndSelection(t *testing.T) {
	m := jiraModel(t)
	defer m.cancel()
	m.useSource(&fakeSource{})
	m.active = model.Reviews
	m.selected = 2
	m.filtering = false
	m.input.SetValue("backend")
	m.jira.query = "already loaded"
	m.jira.active = 2
	m.jira.rows = []jira.Issue{{Key: "APP-1"}, {Key: "APP-2"}}
	m.jira.selected = 1
	m.jira.custom = "project = APP"
	m, _ = sequence(m, "vg")
	if (m.page == pageJira) || m.active != model.Reviews || m.selected != 2 || m.input.Value() != "backend" || m.filtering {
		t.Fatal("GitHub state lost")
	}
	m, _ = sequence(m, "vt")
	if !(m.page == pageJira) || m.jira.active != 2 || m.jira.selected != 1 || m.jira.custom != "project = APP" {
		t.Fatal("Jira state lost")
	}
	if m.jira.generation != 0 {
		t.Fatal("switch triggered unwanted refresh")
	}
	_, cmd := sequence(m, "vt")
	if cmd != nil {
		t.Fatal("already-active shortcut did work")
	}
}
func TestFirstJiraVisitLoadsOnce(t *testing.T) {
	m := jiraModel(t)
	defer m.cancel()
	m.useSource(&fakeSource{})
	m.setPageFlag(pageJira, false)
	m, cmd := sequence(m, "vt")
	if !(m.page == pageJira) || cmd == nil || m.jira.generation != 1 {
		t.Fatal("first visit did not load")
	}
	m, _ = sequence(m, "vg")
	m, cmd = sequence(m, "vt")
	if cmd != nil || m.jira.generation != 1 {
		t.Fatal("return visit restarted in-flight request")
	}
}
func TestMissingProviderKeepsCurrentView(t *testing.T) {
	m := jiraModel(t)
	defer m.cancel()
	m, _ = sequence(m, "vg")
	if !(m.page == pageJira) || m.setup.provider == "" {
		t.Fatal("missing GitHub not explained")
	}
	gh := ready(t)
	gh, _ = sequence(gh, "vt")
	if (gh.page == pageJira) || gh.setup.provider == "" {
		t.Fatal("missing Jira not explained")
	}
}

func TestProviderLettersRemainEditable(t *testing.T) {
	m := jiraModel(t)
	defer m.cancel()
	m.useSource(&fakeSource{})
	m.jira.edit("jql", "JQL: ", "")
	m, _ = update(m, key('g'))
	m, _ = update(m, key('J'))
	if !(m.page == pageJira) || m.jira.input.Value() != "gJ" {
		t.Fatal("shortcut consumed JQL text")
	}
	gh := ready(t)
	gh.filtering = true
	gh.input.Focus()
	gh, _ = update(gh, key('J'))
	gh, _ = update(gh, key('g'))
	if (gh.page == pageJira) || gh.input.Value() != "Jg" {
		t.Fatal("shortcut consumed filter text")
	}
}
