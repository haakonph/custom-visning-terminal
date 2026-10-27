package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/jira"
)

func jiraModel(t *testing.T) Model {
	t.Helper()
	t.Setenv("CUSTOM_VISNING_LINKS", t.TempDir()+"/links.sqlite")
	t.Setenv("CUSTOM_VISNING_TASKS", t.TempDir()+"/tasks.sqlite")
	t.Setenv("CUSTOM_VISNING_CONFIG", t.TempDir()+"/jira.json")
	c, err := jira.New("https://jira.example", "test")
	if err != nil {
		t.Fatal(err)
	}
	return New(context.Background(), nil, nil).WithJira(c, true)
}
func TestJiraStaleResultsAndFailureRetainRows(t *testing.T) {
	m := jiraModel(t)
	defer m.cancel()
	m.jira.generation = 2
	m.jira.rows = []jira.Issue{{Key: "APP-1"}}
	updated, _ := m.Update(jiraResult{generation: 1, rows: []jira.Issue{{Key: "OLD"}}})
	m = updated.(Model)
	if m.jira.rows[0].Key != "APP-1" {
		t.Fatal("stale response replaced rows")
	}
	updated, _ = m.Update(jiraResult{generation: 2, err: errors.New("offline")})
	m = updated.(Model)
	if len(m.jira.rows) != 1 || m.jira.notice != "offline" {
		t.Fatal("failed refresh lost cached rows or error")
	}
}
func TestJiraSavedQueryNotModifiedByFilters(t *testing.T) {
	m := jiraModel(t)
	defer m.cancel()
	m.jira.tabs = append(m.jira.tabs, jira.Query{Name: "Done", JQL: "statusCategory = Done order by updated"})
	m.jira.active = 4
	m.jira.filters.Project = "APP"
	if got := m.jira.currentQuery(); got != "statusCategory = Done order by updated" {
		t.Fatal(got)
	}
}
func TestJiraFilterCancelAndLocalSearch(t *testing.T) {
	m := jiraModel(t)
	defer m.cancel()
	m.jira.filters.Project = "APP"
	m.jira.filterBackup = m.jira.filters
	m.jira.filters.Project = "OTHER"
	m.jira.mode = "filter"
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	if m.jira.filters.Project != "APP" {
		t.Fatal("cancel kept edited filters")
	}
	m.jira.rows = []jira.Issue{{Key: "APP-1", Title: "Login"}, {Key: "APP-2", Title: "Export"}}
	m.jira.input.Prompt = "/ "
	m.jira.input.SetValue("login")
	if len(m.jira.filtered()) != 1 {
		t.Fatal("local filter failed")
	}
	m.width = 30
	m.height = 8
	_ = m.View()
}

func TestJiraSavesAndReloadsCustomQuery(t *testing.T) {
	m := jiraModel(t)
	defer m.cancel()
	m.jira.custom = "project = APP ORDER BY updated ASC"
	m.jira.edit("name", "Lagre søk som: ", "Arbeid")
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	config, err := jira.LoadConfig(m.jira.configPath)
	if err != nil || len(config.Queries) != 1 {
		t.Fatal(config, err)
	}
	if config.Queries[0].JQL != m.jira.custom || len(m.jira.tabs) != 5 {
		t.Fatal("saved query differs")
	}
	m.jira.custom = "statusCategory = Done"
	m.jira.edit("name", "Lagre søk som: ", "Arbeid")
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(m.jira.tabs) != 5 || m.jira.config.Queries[0].JQL != "statusCategory = Done" {
		t.Fatal("named replacement failed")
	}
}

func TestJiraPastedJQLAndModeSwitch(t *testing.T) {
	m := jiraModel(t)
	defer m.cancel()
	m.jira.edit("jql", "JQL: ", "")
	updated, _ := m.Update(tea.PasteMsg{Content: "project = APP ORDER BY updated DESC"})
	m = updated.(Model)
	if m.jira.input.Value() != "project = APP ORDER BY updated DESC" {
		t.Fatal("pasted JQL lost")
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	if m.jira.mode != "" {
		t.Fatal("editor not canceled")
	}
	// GitHub results still finish while Jira is visible.
	m.loading[0] = true
	m, _ = update(m, refreshMsg{view: 0})
	if m.loading[0] {
		t.Fatal("background GitHub refresh was swallowed")
	}
}

func TestJiraTabsAndLocalFilterMatchGitHub(t *testing.T) {
	m := jiraModel(t)
	defer m.cancel()
	m.jira.tabs = append(m.jira.tabs, jira.Query{Name: "Bugs", JQL: "issuetype = Bug"})
	m.jira.input.Prompt = "/ "
	m.jira.input.SetValue("login")
	m, cmd := sequence(m, "h")
	if m.jira.active != 4 || cmd == nil || m.jira.custom != "" {
		t.Fatal("previous tab failed")
	}
	if m.jira.input.Value() != "login" {
		t.Fatal("lost filter")
	}
	if !strings.Contains(m.View().Content, "Bugs") {
		t.Fatal("tab label missing")
	}
	generation := m.jira.generation
	m, cmd = sequence(m, "9")
	if m.jira.active != 4 || cmd != nil || m.jira.generation != generation {
		t.Fatal("legacy number changed view")
	}
	m, _ = sequence(m, "ll")
	if m.jira.active != 1 || m.jira.input.Value() != "login" {
		t.Fatal("tab/filter behavior differs")
	}
}
