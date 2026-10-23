package tui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/jira"
	"github.com/haakonph/custom-visning-terminal/internal/local"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

type folderSource struct {
	fakeSource
	checked      bool
	repositories []string
}

func (s *folderSource) WritableRepositories(context.Context) ([]string, error) {
	if s.repositories != nil {
		return s.repositories, nil
	}
	return []string{"acme/write", "other/write"}, nil
}
func (s *folderSource) CheckWritable(context.Context, string) error {
	s.checked = true
	return errors.New("write access revoked")
}

func TestFoldersNavigationPreferencesAndSearchOwnInput(t *testing.T) {
	m := localModel(t)
	original := m.active
	var cmd tea.Cmd
	m, cmd = sequence(m, "vl")
	if !m.folders.active || cmd == nil || !strings.Contains(m.View().Content, "Lokale mapper") {
		t.Fatal("folder view missing")
	}
	first := local.Folder{Path: filepath.Join(t.TempDir(), "repo one"), Repository: "acme/one"}
	second := local.Folder{Path: filepath.Join(t.TempDir(), "repo two"), Repository: "acme/two"}
	m, _ = update(m, foldersLoaded{rows: []local.Folder{first, second}})
	m, cmd = sequence(m, "af")
	if !m.folders.rows[0].Favorite || cmd == nil {
		t.Fatal("favorite not saved")
	}
	m, _ = update(m, cmd())
	// The refresh contains saved metadata, including the chosen favorite.
	if len(m.folderRows()) != 1 || !m.folderRows()[0].Favorite {
		t.Fatal("favorite did not persist")
	}
	m, cmd = sequence(m, "ah")
	m, _ = update(m, cmd())
	if len(m.folderRows()) != 0 {
		t.Fatal("hidden folder still visible")
	}
	m, _ = sequence(m, "ll")
	if len(m.folderRows()) != 1 {
		t.Fatal("cannot recover hidden folder")
	}
	m, cmd = sequence(m, "ah")
	m, _ = update(m, cmd())
	if len(m.folderRows()) != 0 {
		t.Fatal("unhidden folder still in hidden tab")
	}
	m, _ = sequence(m, "/avte")
	if m.sequence != "" || m.active != original || m.folders.query != "avte" {
		t.Fatal("search dispatched application shortcuts")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.folders.active {
		t.Fatal("Esc did not leave view")
	}
}

func TestIssueFolderPickerCancellationAndPermissionRecheck(t *testing.T) {
	m := localModel(t)
	source := &folderSource{}
	m.source = source
	refs := []model.TaskReference{{Provider: "jira", Key: "APP-12", Title: "Jira task"}}
	cmd := m.startIssueFolder(refs)
	if cmd == nil || !m.folders.picking {
		t.Fatal("picker not started")
	}
	result := cmd()
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m, _ = update(m, result)
	if m.folders.picking || len(m.folders.repositories) != 0 {
		t.Fatal("late result reopened cancelled picker")
	}
	cmd = m.startIssueFolder(refs)
	m, _ = update(m, cmd())
	if len(m.folderRepositories()) != 2 || !strings.Contains(m.View().Content, "APP-12") {
		t.Fatal("repositories missing")
	}
	m, cmd = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || !m.folders.busy {
		t.Fatal("creation not scheduled")
	}
	m, _ = update(m, cmd())
	if !source.checked || m.folders.busy || !strings.Contains(m.folders.notice, "revoked") {
		t.Fatal("permission loss ignored")
	}
	rows, err := m.local.manager.Folders(context.Background())
	if err != nil || len(rows) != 0 {
		t.Fatal("created checkout despite denied write permission", err)
	}
}

func TestFolderResultsArriveWhileSettingsOpen(t *testing.T) {
	m := localModel(t)
	m.settings.active = true
	m.folders.busy = true
	m, _ = update(m, folderCreated{folder: local.Folder{Path: "/tmp/test", Repository: "acme/repo", IssueKey: "APP-12"}})
	if m.folders.busy || len(m.folders.rows) != 1 || !m.settings.active {
		t.Fatal("settings lost background creation result")
	}
}

func TestJiraFolderShortcutsAndMultipleSelection(t *testing.T) {
	for _, shortcut := range []string{"aw", "ee"} {
		m := localModel(t)
		client, err := jira.New("https://jira.example.com", "test-token")
		if err != nil {
			t.Fatal(err)
		}
		m = m.WithJira(client, true)
		m.source = &folderSource{}
		m.jira.rows = []jira.Issue{{Key: "APP-12", Title: "Task", URL: client.IssueURL("APP-12")}}
		m.jira.query = m.jira.currentQuery()
		next, cmd := sequence(m, shortcut)
		if cmd == nil || !next.folders.picking || next.folders.issue.Key != "APP-12" || next.folders.openAfterCreate != (shortcut == "ee") {
			t.Fatalf("%s did not open repository picker: %#v", shortcut, next.folders)
		}
	}
	m := localModel(t)
	m.source = &folderSource{}
	refs := []model.TaskReference{{Provider: "jira", Key: "APP-12"}, {Provider: "jira", Key: "APP-13"}}
	if cmd := m.startIssueFolder(refs); cmd != nil || m.folders.picking {
		t.Fatal("multiple issues created one ambiguous folder")
	}
}

func TestIssueFolderEditorResultKeepsFolderOnLaunchFailure(t *testing.T) {
	for _, openErr := range []error{nil, errors.New("editor unavailable")} {
		m := localModel(t)
		folder := local.Folder{Path: "/tmp/issue-worktree", Repository: "acme/api", IssueKey: "APP-12"}
		m.folders.busy, m.folders.picking = true, true
		m, _ = update(m, folderCreated{folder: folder, openRequested: true, openErr: openErr})
		if m.folders.busy || m.folders.picking || !m.folders.active || len(m.folderRows()) != 1 {
			t.Fatal("lost created folder after editor result")
		}
		want := "Mappe åpnet i editor"
		if openErr != nil {
			want = "editor unavailable"
		}
		if !strings.Contains(m.folders.notice, want) {
			t.Fatal(m.folders.notice)
		}
	}
}
