package tui

import (
	"charm.land/bubbles/v2/textinput"
	"github.com/haakonph/custom-visning-terminal/internal/jira"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"testing"
	"time"
)

func TestDomainResultsApplyBehindOverlays(t *testing.T) {
	for _, overlay := range []string{"settings", "wizard", "dialog", "task-editor"} {
		t.Run(overlay, func(t *testing.T) {
			m := ready(t)
			m.useSource(&autoMergeSource{})
			id := m.filtered()[0].Key()
			next, run := m.startAutoMerge([]model.TaskReference{{Provider: "github", URL: id}}, true)
			m = next.(Model)
			switch overlay {
			case "settings":
				m.settings.active = true
			case "wizard":
				m = m.startSetupWizard("github", nil)
			case "dialog":
				m.dialog = &entityDialog{mode: "snapshot"}
			case "task-editor":
				m.page = pageTasks
				m.tasks.editing = true
				m.tasks.input.SetValue("Unsubmitted draft")
			}
			m, _ = update(m, run())
			if p, _ := m.prs.Lookup(id); p.AutoMerge != "ON" || m.prs.AutoMergeBusy() {
				t.Fatal("completion swallowed by overlay", overlay)
			}
			if overlay == "task-editor" && m.tasks.input.Value() != "Unsubmitted draft" {
				t.Fatal("background result changed draft")
			}
		})
	}
}
func TestOldIntegrationResultsCannotAlterReplacementSessions(t *testing.T) {
	t.Run("jira", func(t *testing.T) {
		m := ready(t)
		m.jiraSession.Configure(m.ctx)
		token := m.jiraSession.Begin("query:q")
		m.jiraSession.Configure(m.ctx)
		m.jira = &jiraState{generation: 1, query: "q", loading: true, rows: []jira.Issue{{Key: "NEW-1"}}}
		m, _ = update(m, jiraResult{token: token, generation: 1, query: "q", rows: []jira.Issue{{Key: "OLD-1"}}, at: time.Now()})
		if !m.jira.loading || m.jira.rows[0].Key != "NEW-1" {
			t.Fatal("old account updated replacement")
		}
	})
	t.Run("copilot", func(t *testing.T) {
		m := ready(t)
		m.copilotSession.Configure(m.ctx)
		token := m.copilotSession.Begin("refresh")
		m.copilotSession.Configure(m.ctx)
		m.copilot = &copilotState{loading: true, input: textinput.New(), sessions: []model.CopilotSession{{ID: "new"}}}
		m, _ = update(m, copilotResult{token: token, sessions: []model.CopilotSession{{ID: "old"}}})
		if !m.copilot.loading || m.copilot.sessions[0].ID != "new" {
			t.Fatal("old root updated replacement")
		}
	})
	t.Run("chat", func(t *testing.T) {
		m := ready(t)
		m.chatSession.Configure(m.ctx)
		token := m.chatSession.Begin("operation")
		m.chatSession.Configure(m.ctx)
		m.mmReader = &mattermostReader{busy: true, data: model.MattermostData{Server: "https://new.example"}}
		m, _ = update(m, mmResult{token: token, data: model.MattermostData{Server: "https://old.example"}})
		if !m.mmReader.busy || m.mmReader.data.Server != "https://new.example" {
			t.Fatal("old chat response accepted")
		}
	})
	t.Run("github", func(t *testing.T) {
		m := ready(t)
		token := m.prs.Begin("bootstrap")
		m.configureGitHub(&fakeSource{}, nil)
		m, _ = update(m, bootMsg{token: token, snapshot: model.Snapshot{User: "old"}})
		if m.prs.Snapshot().User != "" || !m.booting {
			t.Fatal("old bootstrap accepted")
		}
	})
}
func TestCacheWritesStayOrderedAcrossSessionReplacement(t *testing.T) {
	m := ready(t)
	store := &fakeCache{}
	m.prs.SetCache(store)
	oldWrite := m.persist()
	m.configureGitHub(&fakeSource{}, store)
	m.prs.Restore(model.Snapshot{Version: 1, User: "new-account"})
	if m.persist() != nil || !m.dirty {
		t.Fatal("replacement bypassed existing writer")
	}
	m, nextWrite := update(m, oldWrite())
	if nextWrite == nil {
		t.Fatal("replacement snapshot never persisted")
	}
	m, _ = update(m, nextWrite())
	if store.snap.User != "new-account" || m.saving {
		t.Fatal("old cache write won")
	}
}
