package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestRepositoryViewWithoutLocalSetup(t *testing.T) {
	m := ready(t)
	m.source = &folderSource{}
	var cmd tea.Cmd
	m, cmd = sequence(m, "vp")
	if cmd == nil || !m.folders.repositoryView || m.local != nil {
		t.Fatal("repository view requires local setup")
	}
	m, _ = update(m, cmd())
	if !strings.Contains(m.View().Content, "acme/write") || !strings.Contains(m.View().Content, "2 av 2") || strings.Contains(m.View().Content, "JIRA") {
		t.Fatal(m.View().Content)
	}
	m, _ = sequence(m, "/OTHER")
	if m.sequence != "" || len(m.folderRepositories()) != 1 {
		t.Fatal("search dispatched shortcuts or ignored case")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, cmd = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || m.folders.busy {
		t.Fatal("Enter should open GitHub without creating a checkout")
	}
	m, cmd = sequence(m, "au")
	if cmd == nil {
		t.Fatal("refresh missing")
	}
	m, _ = update(m, folderReposLoaded{err: errors.New("SSO required"), generation: m.folders.generation})
	if len(m.folderRepositories()) != 1 || !strings.Contains(m.View().Content, "SSO required") {
		t.Fatal("refresh lost cached rows or error")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.folders.active {
		t.Fatal("Esc did not leave")
	}
}

func TestRepositoryViewIgnoresLateResultsAndClampsSelection(t *testing.T) {
	m := ready(t)
	m.source = &folderSource{}
	m, cmd := sequence(m, "vp")
	result := cmd()
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m, _ = update(m, result)
	if m.folders.repositoryView || len(m.folders.repositories) != 0 {
		t.Fatal("late response changed closed view")
	}
	m, cmd = sequence(m, "vp")
	m, _ = update(m, cmd())
	m, _ = sequence(m, "j")
	m, _ = update(m, folderReposLoaded{rows: []string{"other/write"}, generation: m.folders.generation})
	if m.folders.selected != 0 {
		t.Fatal("selection outside refreshed list")
	}
	m, _ = update(m, folderReposLoaded{generation: m.folders.generation})
	if !strings.Contains(m.View().Content, "Ingen repoer med skrivetilgang") {
		t.Fatal(m.View().Content)
	}
	m, cmd = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("empty list opened browser")
	}
}

func TestRepositoryViewMissingGitHubAndSearchIsolation(t *testing.T) {
	m := ready(t)
	m, cmd := sequence(m, "vp")
	if cmd != nil || !strings.Contains(m.View().Content, "GitHub må være konfigurert") {
		t.Fatal(m.View().Content)
	}
	m, _ = sequence(m, "/avpe")
	if m.folders.query != "avpe" || !m.folders.repositoryView || m.sequence != "" {
		t.Fatal("search triggered commands")
	}
}

func TestRepositoryViewSwitchCancelsPendingLoad(t *testing.T) {
	m := ready(t)
	m.source = &folderSource{}
	m, cmd := sequence(m, "vp")
	old := cmd()
	m, _ = sequence(m, "vg")
	m, cmd = sequence(m, "vp")
	if cmd == nil {
		t.Fatal("reopening stuck on abandoned load")
	}
	m, _ = update(m, old)
	if !m.folders.repoLoading || len(m.folders.repositories) != 0 {
		t.Fatal("old result replaced new request")
	}
	m, _ = update(m, cmd())
	if m.folders.repoLoading || len(m.folderRepositories()) != 2 {
		t.Fatal("fresh result missing")
	}
}
