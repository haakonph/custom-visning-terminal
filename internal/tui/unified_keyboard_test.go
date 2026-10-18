package tui

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/jira"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/storage"
)

func unifiedModel(t *testing.T) Model {
	t.Helper()
	m := readerModel(t)
	m.mmReader.active = false
	m.mmReader.entities = m.entities
	m.mmReader.snoozes = m.snoozes
	client, _ := jira.New("https://jira.example/context", "secret")
	m = m.WithJira(client, false).WithCopilot(testCopilot(t), false)
	m.snapshot.Datasets[0].PRs[0].Title = "APP-1 Fix"
	m.snapshot.Datasets[0].PRs[0].HeadRef = "feature/APP-1"
	m.jira.rows = []jira.Issue{{Key: "APP-1", URL: client.IssueURL("APP-1"), Title: "Issue"}, {Key: "APP-2", URL: client.IssueURL("APP-2"), Title: "Other"}}
	m.jira.query = m.jira.currentQuery()
	m.copilot.sessions = []model.CopilotSession{{ID: "ai1", Name: "Session", Jira: "APP-1", Repository: "acme/api", Branch: "feature/APP-1"}, {ID: "ai2", Name: "Other session"}}
	m.tasks.items = []model.Task{{ID: "task1", Title: "Work", References: []model.TaskReference{prRef(m.snapshot.Datasets[0].PRs[0]), jiraRef("APP-2", m)}}}
	return m
}
func TestRemovedNavigationKeysAcrossAllProviders(t *testing.T) {
	old := []string{"g", "J", "c", "T", "M", "m", "o", "O", "R", "E", "D", "A", "n", "f", ":", "s", ",", "P", "i", "+", "-", "u", "H", "[", "]", "q", "1", "2", "3", "4", "5", "6", "7", "8", "9"}
	for _, page := range []string{"g", "t", "a", "c", "o"} {
		t.Run(page, func(t *testing.T) {
			m := unifiedModel(t)
			m, _ = sequence(m, "v"+page)
			for _, k := range old {
				before := m.visibleProvider()
				next, cmd := sequence(m, k)
				if cmd != nil || next.sequence != "" || next.visibleProvider() != before || next.ctx.Err() != nil || next.settings.active || next.tasks.editing || next.detail != nil || next.dialog != nil {
					t.Fatalf("legacy key %q still acts on %s", k, before)
				}
				m = next
			}
			for _, code := range []rune{tea.KeyTab, tea.KeyEnter} {
				_, cmd := update(m, tea.KeyPressMsg{Code: code})
				if cmd != nil {
					t.Fatal("legacy control key acts")
				}
			}
		})
	}
}
func TestAllFiveLinkTargetsPersistAndAreNavigable(t *testing.T) {
	for _, thing := range []string{"o", "g", "t", "a", "c"} {
		t.Run(thing, func(t *testing.T) {
			m := unifiedModel(t)
			origin := prRef(m.snapshot.Datasets[0].PRs[1])
			m.selected = 1
			m, _ = sequence(m, "t"+thing)
			if m.dialog == nil || m.dialog.mode != "link" {
				t.Fatal("link chooser missing")
			}
			if thing == "g" {
				m.dialog.selected = 0
			}
			target := m.dialog.filtered()[m.dialog.selected]
			m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
			if m.dialog != nil || len(m.entities.data.Links) != 1 {
				t.Fatal("link not saved")
			}
			reloaded := newEntities()
			if reloaded.err != nil || len(reloaded.data.Links) != 1 {
				t.Fatal("link not reloaded", reloaded.err)
			}
			m.entities = reloaded
			got := m.relatedReferences([]model.TaskReference{origin}, provider(thing))
			found := false
			for _, r := range got {
				if refID(r) == refID(target) {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing relation to %s", thing)
			}
		})
	}
}
func TestActionTraversalDoesNotIncludeUnrelatedTaskIssue(t *testing.T) {
	m := unifiedModel(t)
	m, _ = sequence(m, "at")
	if len(m.sequenceRefs) != 1 || m.sequenceRefs[0].Key != "APP-1" {
		t.Fatalf("action traversed unrelated task issue: %v", m.sequenceRefs)
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m, _ = sequence(m, "aot")
	if len(m.sequenceRefs) != 1 || m.sequenceRefs[0].Key != "APP-2" {
		t.Fatalf("explicit task path not followed: %v", m.sequenceRefs)
	}
}
func TestAllProvidersSupportSelectionAndSnooze(t *testing.T) {
	for _, page := range []string{"g", "t", "a", "c", "o"} {
		t.Run(page, func(t *testing.T) {
			m := unifiedModel(t)
			m.snoozes.store.Path = t.TempDir() + "/snooze.sqlite"
			m, _ = sequence(m, "v"+page)
			m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})
			count := len(m.currentReferences())
			if count == 0 {
				t.Fatal("no selection")
			}
			m, _ = sequence(m, "z2")
			if !strings.Contains(m.View().Content, "Snooze:") {
				t.Fatal("snooze prompt hidden")
			}
			m, _ = sequence(m, "h")
			if len(m.snoozes.entries) != count || m.hasChosen() {
				t.Fatal("snooze ignored selection")
			}
			loaded, err := m.snoozes.store.Load()
			if err != nil || len(loaded) != count {
				t.Fatal("snooze not persisted", err)
			}
		})
	}
}
func TestArchivesSurviveRestartForAllProviders(t *testing.T) {
	for _, page := range []string{"g", "t", "a", "c", "o"} {
		t.Run(page, func(t *testing.T) {
			m := unifiedModel(t)
			m, _ = sequence(m, "v"+page)
			origin := m.currentReferences()[0]
			m, _ = sequence(m, "ak")
			m.entities = newEntities()
			if origin.Provider == "task" {
				tasks := newTasks()
				if !tasks.items[0].Archived {
					t.Fatal("task archive not persisted")
				}
				m.tasks = tasks
			} else if !m.entities.archived(origin.Provider, origin.URL) {
				t.Fatal("archive not persisted")
			}
			m, _ = sequence(m, "vb")
			if m.dialog == nil {
				t.Fatal("archive not visible")
			}
			m, _ = sequence(m, "ak")
			if m.entities.archived(origin.Provider, origin.URL) {
				t.Fatal("restore failed")
			}
			if origin.Provider == "task" && newTasks().items[0].Archived {
				t.Fatal("task restore not saved")
			}
		})
	}
}
func TestLinkCancelAndWriteFailureNeverCreateEdges(t *testing.T) {
	m := unifiedModel(t)
	m, _ = sequence(m, "ta")
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if len(m.entities.data.Links) != 0 {
		t.Fatal("cancel wrote links")
	}
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	m.entities.path = filepath.Join(blocked, "links.sqlite")
	m, _ = sequence(m, "ta")
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(m.entities.data.Links) != 0 || !strings.Contains(m.sequenceNotice, "Kunne ikke lagre") {
		t.Fatal("failed save changed links")
	}
}
func TestAIActionTargetAndGenericMessage(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	m := unifiedModel(t).WithMattermost(&fakeMessageSender{})
	m, _ = sequence(m, "aas")
	if !m.mattermost.active || !strings.Contains(m.mattermost.text, "Session") {
		t.Fatal("AI action target unavailable")
	}
}
func TestLinkedTaskDeletionRemovesPersistedRelations(t *testing.T) {
	m := unifiedModel(t)
	m, _ = sequence(m, "to")
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = sequence(m, "vo")
	m, _ = sequence(m, "ad")
	if len(m.tasks.items) != 0 || len(newEntities().data.Links) != 0 {
		t.Fatal("deletion left dangling links")
	}
}
func TestEntityStorageRoundTrip(t *testing.T) {
	path := t.TempDir() + "/links.sqlite"
	original := model.EntityState{Links: []model.EntityLink{{Left: model.TaskReference{Provider: "github", URL: "https://github.com/a/b/pull/1", Key: "a/b#1", Title: "PR"}, Right: model.TaskReference{Provider: "copilot", URL: "copilot://session/id", Key: "id", Title: "AI"}}}, Archived: []model.TaskReference{{Provider: "mattermost", URL: "https://chat.example/_redirect/pl/id", Key: "id", Title: "Message"}}}
	if err := storage.Save(path, original); err != nil {
		t.Fatal(err)
	}
	var loaded model.EntityState
	if err := storage.Load(path, &loaded); err != nil || !reflect.DeepEqual(original, loaded) {
		t.Fatalf("%#v %v", loaded, err)
	}
}
func TestContextSnapshotSurvivesBackgroundRefresh(t *testing.T) {
	m := unifiedModel(t)
	m, _ = sequence(m, "at")
	m.snapshot.Datasets[0].PRs[0].Title = "APP-99 changed"
	keys := m.actionJiraKeys()
	if len(keys) != 1 || keys[0] != "APP-1" {
		t.Fatal("pending command changed targets")
	}
}
func TestBrowserPreflightsUnsupportedTargets(t *testing.T) {
	m := unifiedModel(t)
	calls := 0
	m.open = func(context.Context, string) error { calls++; return nil }
	refs := []model.TaskReference{prRef(m.snapshot.Datasets[0].PRs[0]), copilotRef(m.copilot.sessions[0])}
	next, cmd := m.openReferences(refs, false)
	if cmd != nil || calls != 0 || !strings.Contains(next.(Model).sequenceNotice, "AI-sesjoner") {
		t.Fatal("partial browser action before preflight")
	}
}

func TestSnoozeAndSettingsOwnInputOverChat(t *testing.T) {
	m := unifiedModel(t)
	m.snoozes.store.Path = t.TempDir() + "/snooze.sqlite"
	m, _ = sequence(m, "vcvs")
	if !m.settings.active || !strings.Contains(m.View().Content, "INNSTILLINGER") {
		t.Fatal("chat obscured settings")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m, _ = sequence(m, "z3")
	if m.snoozes.digits != "3" || !strings.Contains(m.View().Content, "Snooze:") {
		t.Fatal("chat stole snooze input")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if len(m.snoozes.entries) != 0 {
		t.Fatal("cancel wrote snooze")
	}
}
func TestStaleEditorTargetNeverFallsBackToHighlightedPR(t *testing.T) {
	m := unifiedModel(t)
	next, cmd := m.openReferences([]model.TaskReference{{Provider: "github", URL: "https://github.com/acme/api/pull/999", Key: "missing"}}, true)
	if cmd != nil || next.(Model).sequenceNotice == "" {
		t.Fatal("missing target fell back to active row")
	}
}
func TestRenameAndDeleteUseExplicitRelatedTask(t *testing.T) {
	m := unifiedModel(t)
	m, _ = sequence(m, "aoe")
	if !m.tasks.editing || m.tasks.items[m.tasks.selected].ID != "task1" {
		t.Fatal("could not rename related task")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m, _ = sequence(m, "vg")
	m, _ = sequence(m, "aod")
	if len(m.tasks.items) != 0 {
		t.Fatal("could not delete related task")
	}
}

func TestSelectionsInOtherProvidersDoNotBecomeTaskReferenceSelections(t *testing.T) {
	m := unifiedModel(t)
	m, _ = sequence(m, "va vod")
	refs := m.currentReferences()
	if len(refs) != 1 || refs[0].Provider != "github" {
		t.Fatalf("hidden selection leaked: %v", refs)
	}
}

func TestSubViewsReplaceDeferredViewAndOwnInput(t *testing.T) {
	for _, command := range []string{"vs", "vi", "vf", "vj", "vk"} {
		t.Run(command, func(t *testing.T) {
			m := unifiedModel(t)
			m.snoozes.active = true
			m, _ = sequence(m, command)
			switch command {
			case "vs":
				if !m.settings.active {
					t.Fatal("settings not opened")
				}
			case "vi":
				if m.visibleProvider() != "copilot" || !m.copilot.diagnosticsOpen {
					t.Fatal("diagnostics hidden")
				}
			case "vf", "vj":
				if m.visibleProvider() != "jira" || m.jira.mode == "" {
					t.Fatal("Jira editor hidden")
				}
			case "vk":
				if m.visibleProvider() != "mattermost" || !m.mmReader.choosing {
					t.Fatal("channel picker hidden")
				}
			}
		})
	}
}
func TestNaturalAIRelationWorksWithoutJiraConnection(t *testing.T) {
	m := unifiedModel(t)
	m.jira = nil
	m.copilot.sessions[0].Repository = ""
	m.copilot.sessions[0].Branch = ""
	origin := prRef(m.snapshot.Datasets[0].PRs[0])
	related := m.relatedReferences([]model.TaskReference{origin}, "copilot")
	if len(related) != 1 || related[0].Key != "ai1" {
		t.Fatal("Jira credentials required for local AI relationship")
	}
}
func TestResetJiraFiltersAndPreviousPageUsePrefixes(t *testing.T) {
	m := unifiedModel(t)
	m, _ = sequence(m, "vt")
	m.jira.filters = jira.Filters{Project: "APP"}
	m.jira.custom = "project = APP"
	m, _ = sequence(m, "ax")
	if m.jira.filters.Project != "" || m.jira.custom != "" {
		t.Fatal("filters not reset")
	}
	m, _ = sequence(m, "vo")
	m, _ = sequence(m, "vr")
	if m.visibleProvider() != "jira" {
		t.Fatal("previous view lost")
	}
}

func TestStoredAIReferenceWithoutProviderDoesNotRecurse(t *testing.T) {
	m := unifiedModel(t)
	m.copilot = nil
	_, cmd := m.openReferences([]model.TaskReference{{Provider: "copilot", URL: "copilot://session/missing", Key: "missing"}}, true)
	if cmd != nil {
		t.Fatal("opened unconfigured AI")
	}
}

func TestDeferredSearchAndBulkRestoreRespectSelection(t *testing.T) {
	m := unifiedModel(t)
	m.snoozes.store.Path = t.TempDir() + "/snooze.sqlite"
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})
	m, _ = sequence(m, "z2hvz/")
	if m.dialog == nil || !m.dialog.searching {
		t.Fatal("deferred search unavailable")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})
	m, _ = sequence(m, "az")
	if len(m.snoozes.entries) != 0 || len(m.filtered()) != 2 {
		t.Fatal("restore ignored marked rows")
	}
}
func TestArchivedDetailsDoNotSelectAnotherVisibleRow(t *testing.T) {
	m := unifiedModel(t)
	target := m.currentReferences()[0]
	m, _ = sequence(m, "ak")
	m, _ = sequence(m, "vb")
	m, _ = sequence(m, "d")
	if m.dialog == nil || m.dialog.mode != "snapshot" || refID(m.currentReferences()[0]) != refID(target) {
		t.Fatal("archived details fell back to a different row")
	}
	m, _ = sequence(m, "ak")
	if m.entities.archived(target.Provider, target.URL) {
		t.Fatal("cannot restore from archived details")
	}
}

func TestCreateTaskFromEveryProviderSwitchesAndKeepsEditorInput(t *testing.T) {
	for _, thing := range []string{"g", "t", "a", "c", "o"} {
		t.Run(thing, func(t *testing.T) {
			m := unifiedModel(t)
			m, _ = sequence(m, "v"+thing+"an")
			if m.visibleProvider() != "task" {
				t.Fatal("new task hidden behind source provider")
			}
			if !m.tasks.editing {
				m, _ = sequence(m, "ae")
			}
			m.tasks.input.SetValue("")
			m, _ = sequence(m, "atfqgc")
			if !m.tasks.editing || m.tasks.input.Value() != "atfqgc" || m.sequence != "" {
				t.Fatal("source provider stole task editor input")
			}
		})
	}
}
func TestSpecialViewCommandsRememberSourcePage(t *testing.T) {
	for _, command := range []string{"vi", "vf", "vj"} {
		m := unifiedModel(t)
		m, _ = sequence(m, command)
		if m.jira != nil {
			m.jira.mode = ""
		}
		m, _ = sequence(m, "vr")
		if m.visibleProvider() != "github" {
			t.Fatalf("%s lost previous page", command)
		}
	}
}

func TestOpenDetailsKeepActionTargetWhenListChanges(t *testing.T) {
	m := unifiedModel(t)
	m, _ = sequence(m, "d")
	expected := m.detail.pr.URL
	m.snapshot.Datasets[0].PRs = m.snapshot.Datasets[0].PRs[1:]
	refs := m.currentReferences()
	if len(refs) != 1 || refs[0].URL != expected {
		t.Fatal("detail command fell back to another PR")
	}
	m.detail = nil
	m.jiraActive = true
	m, _ = sequence(m, "d")
	key := m.jira.detail.Key
	m.jira.rows = m.jira.rows[1:]
	refs = m.currentReferences()
	if len(refs) != 1 || refs[0].Key != key {
		t.Fatal("detail command fell back to another Jira issue")
	}
}

func TestAIAndChatAreUnambiguousActionTargets(t *testing.T) {
	for _, thing := range []string{"a", "c"} {
		for _, action := range []string{"s", "k"} {
			t.Run(thing+action, func(t *testing.T) {
				t.Setenv("PATH", t.TempDir())
				m := unifiedModel(t).WithMattermost(&fakeMessageSender{})
				m, cmd := sequence(m, "a"+thing)
				if cmd != nil || m.sequence != "a"+thing || len(m.sequenceRefs) == 0 || !allProvider(m.sequenceRefs, provider(thing)) || m.mattermost.active || len(m.entities.data.Archived) != 0 {
					t.Fatal("entity key executed an action instead of following its relation")
				}
				targets := append([]model.TaskReference(nil), m.sequenceRefs...)
				m, _ = sequence(m, action)
				if action == "s" {
					if !m.mattermost.active || !strings.Contains(m.mattermost.text, targets[0].Title) {
						t.Fatal("message did not use related targets")
					}
				} else if !reflect.DeepEqual(m.entities.data.Archived, targets) {
					t.Fatal("archive did not use related targets")
				}
			})
		}
	}
}

func TestJiraQuerySaveDoesNotCollideWithSend(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	m := unifiedModel(t).WithMattermost(&fakeMessageSender{})
	m, _ = sequence(m, "vtaq")
	if m.jira.mode != "name" || m.mattermost.active {
		t.Fatal("aq did not open query name input")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m, _ = sequence(m, "as")
	if !m.mattermost.active || m.jira.mode != "" {
		t.Fatal("as did not compose a message from Jira")
	}
}

func TestChatScrollDoesNotCollideWithArchive(t *testing.T) {
	m := unifiedModel(t)
	m, _ = sequence(m, "vc")
	m.height = 20
	m, _ = sequence(m, "a")
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	if m.mmReader.scroll != 10 {
		t.Fatal("a down did not scroll half a page")
	}
	m, _ = sequence(m, "a")
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyUp})
	if m.mmReader.scroll != 0 {
		t.Fatal("a up did not scroll half a page")
	}
	m, _ = sequence(m, "ak")
	if len(m.entities.data.Archived) != 1 || m.entities.data.Archived[0].Provider != "mattermost" {
		t.Fatal("ak did not archive chat message")
	}
}
