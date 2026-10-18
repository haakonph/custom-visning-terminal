package tui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/jira"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestPatternSettingsSaveReloadAndReset(t *testing.T) {
	for _, jiraActive := range []bool{false, true} {
		m := jiraModel(t)
		defer m.cancel()
		m.jiraActive = jiraActive
		query := jira.Query{Name: "Mine", JQL: "assignee = currentUser()"}
		if err := jira.SaveConfig(jira.ConfigPath(), jira.Config{Queries: []jira.Query{query}}); err != nil {
			t.Fatal(err)
		}
		m, _ = update(m, key('P'))
		if !m.patternSettings.active {
			t.Fatal("settings did not open")
		}
		m.patternSettings.input.SetValue(`KAKE-[0-9]{3,5}`)
		m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
		if m.patternSettings.active {
			t.Fatal(m.patternSettings.notice)
		}
		config, err := jira.LoadConfig(jira.ConfigPath())
		if err != nil || config.Pattern != `KAKE-[0-9]{3,5}` || len(config.Queries) != 1 || config.Queries[0] != query {
			t.Fatalf("config not preserved: %+v %v", config, err)
		}
		reloaded := New(context.Background(), nil, nil).WithJiraPattern(nil)
		defer reloaded.cancel()
		p := model.PullRequest{Title: "KAKE-123 ABC-12"}
		if got := p.JiraKeys(reloaded.jiraPattern); len(got) != 1 || got[0] != "KAKE-123" {
			t.Fatal(got)
		}
		m, _ = update(m, key('P'))
		m.patternSettings.input.SetValue("")
		m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
		if m.jiraPattern != nil {
			t.Fatal("reset failed")
		}
	}
}

func TestPatternSettingsInvalidCancelAndWriteFailure(t *testing.T) {
	m := jiraModel(t)
	defer m.cancel()
	m, _ = update(m, key('P'))
	m.patternSettings.input.SetValue("[")
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.patternSettings.active || m.patternSettings.notice == "" || m.jiraPattern != nil {
		t.Fatal("invalid pattern accepted")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.patternSettings.active || m.jiraPattern != nil {
		t.Fatal("cancel failed")
	}
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JIRA_CONFIG", filepath.Join(blocker, "jira.json"))
	m, _ = update(m, key('P'))
	m.patternSettings.input.SetValue(`KAKE-[0-9]{3,5}`)
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.patternSettings.active || m.patternSettings.notice == "" || m.jiraPattern != nil {
		t.Fatal("write failure changed active pattern")
	}
}
