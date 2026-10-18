package tui

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/jira"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func sequence(m Model, text string) (Model, tea.Cmd) {
	var cmd tea.Cmd
	for _, r := range text {
		m, cmd = update(m, key(r))
	}
	return m, cmd
}

type keyboardTransport func(*http.Request) (*http.Response, error)

func (f keyboardTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestChainedJiraActionBeforeAndAfterLoading(t *testing.T) {
	for _, fast := range []bool{true, false} {
		t.Run(fmt.Sprint(fast), func(t *testing.T) {
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			m := ready(t)
			client, _ := jira.New("https://jira.example/context", "secret")
			m = m.WithJira(client, false)
			m.snapshot.Datasets[model.Open].PRs[0].Title = "APP-1 APP-2"
			m.snapshot.Datasets[model.Open].PRs[1].Title = "APP-2"
			m, _ = update(m, key(' '))
			m, _ = update(m, key('j'))
			m, _ = update(m, key(' '))
			original := http.DefaultTransport
			t.Cleanup(func() { http.DefaultTransport = original })
			gets, posts := []string{}, []string{}
			http.DefaultTransport = keyboardTransport(func(r *http.Request) (*http.Response, error) {
				if r.Header.Get("Authorization") != "Bearer secret" {
					t.Fatal("missing auth")
				}
				body := `{"transitions":[{"id":"17","name":"Send to QA","to":{"name":"QA"}}]}`
				if r.Method == "GET" {
					gets = append(gets, r.URL.Path)
				} else {
					posts = append(posts, r.URL.Path)
					data, _ := io.ReadAll(r.Body)
					if !strings.Contains(string(data), `"id":"17"`) {
						t.Fatalf("workflow ID not used: %s", data)
					}
					body = ""
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})
			m, load := sequence(m, "atf")
			if load == nil || m.jiraAction == nil {
				t.Fatal("not loading")
			}
			if fast {
				m, _ = sequence(m, "q")
			}
			m, apply := update(m, load())
			if !fast {
				m, apply = sequence(m, "q")
			}
			if apply == nil {
				t.Fatal("QA not scheduled")
			}
			m, _ = update(m, apply())
			if m.jiraActive || m.jiraAction != nil || len(gets) != 2 || len(posts) != 2 || !strings.Contains(m.sequenceNotice, "APP-1, APP-2") {
				t.Fatalf("wrong targets/state: %v %v %s", gets, posts, m.sequenceNotice)
			}
		})
	}
}
func TestSequenceCancellationAndStaleJiraResult(t *testing.T) {
	m := ready(t)
	client, _ := jira.New("https://jira.example", "secret")
	m = m.WithJira(client, false)
	m.snapshot.Datasets[0].PRs[0].Title = "APP-1"
	m, _ = sequence(m, "atf")
	request := m.jiraAction
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m, cmd := update(m, jiraActionMsg{request: request, transitions: map[string][]jira.Transition{}})
	if m.jiraAction != nil || cmd != nil || m.sequence != "" {
		t.Fatal("cancelled action resumed")
	}
	m, _ = sequence(m, "at")
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m, _ = sequence(m, "j")
	if m.selected != 1 || m.sequence != "" {
		t.Fatal("Esc did not restore navigation")
	}
}
func TestAmbiguousTransitionDoesNotExecute(t *testing.T) {
	m := ready(t)
	a := jira.Transition{ID: "1", Name: "QA"}
	b := jira.Transition{ID: "2", Name: "QA"}
	state := &jiraActionState{keys: []string{"APP-1"}, loading: true, shortcut: "q"}
	m.jiraAction = state
	m, cmd := update(m, jiraActionMsg{request: state, transitions: map[string][]jira.Transition{"APP-1": {a, b}}})
	if cmd != nil || m.jiraAction != nil || !strings.Contains(m.sequenceNotice, "entydige") {
		t.Fatal("ambiguous workflow executed")
	}
}
func TestKeyboardSelectionTabsAndTextInput(t *testing.T) {
	m := ready(t)
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})
	if len(m.chosen) != 2 || m.selected != 1 {
		t.Fatal("range selection")
	}
	m, _ = sequence(m, "lx")
	if m.active != model.Drafts || len(m.chosen) != 0 {
		t.Fatal("tabs/clear selection")
	}
	m, _ = sequence(m, "h/")
	m, _ = sequence(m, "atfqvetor")
	if !m.filtering || m.sequence != "" || m.input.Value() != "atfqvetor" {
		t.Fatal("shortcut intercepted filter")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m, _ = sequence(m, "v")
	if !strings.Contains(m.View().Content, "Tastsekvens: v") {
		t.Fatal("prefix hint missing")
	}
}
func TestJiraSelectionLinkingAndDetails(t *testing.T) {
	t.Setenv("CUSTOM_VISNING_TASKS", t.TempDir()+"/tasks.json")
	m := jiraModel(t)
	m.jira.rows = []jira.Issue{{Key: "APP-1", Title: "One", URL: "https://jira.example/browse/APP-1", Description: "Details"}, {Key: "APP-2", Title: "Two", URL: "https://jira.example/browse/APP-2"}}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})
	m, _ = sequence(m, "tn")
	if len(m.tasks.items) != 1 || len(m.tasks.items[0].References) != 2 {
		t.Fatal("Jira multiselect not linked")
	}
	m.tasks.active = false
	m.jira.selected = 0
	m, _ = sequence(m, "d")
	if m.jira.detail == nil || !strings.Contains(m.View().Content, "Details") {
		t.Fatal("Jira details")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m, _ = sequence(m, "x")
	if len(m.jiraChosen) != 0 {
		t.Fatal("Jira selection not cleared")
	}
}
func TestTaskArchiveRestoreAndDetailsDoNotDelete(t *testing.T) {
	t.Setenv("CUSTOM_VISNING_TASKS", t.TempDir()+"/tasks.json")
	m := New(context.Background(), nil, nil)
	defer m.cancel()
	m.tasks.active = true
	m.tasks.items = []model.Task{{Title: "Keep", References: []model.TaskReference{{Key: "APP-1"}}}}
	m, _ = sequence(m, "d")
	if len(m.tasks.items[0].References) != 1 || !m.tasks.details {
		t.Fatal("d deleted reference")
	}
	m.tasks.details = false
	m, _ = sequence(m, "aa")
	if len(m.tasks.visibleTasks()) != 0 {
		t.Fatal("not archived")
	}
	m, _ = sequence(m, "Aaa")
	m, _ = sequence(m, "A")
	if len(m.tasks.visibleTasks()) != 1 || m.tasks.items[0].Archived {
		t.Fatal("not restored")
	}
}

func TestTaskShortcutsUseVisibleProvider(t *testing.T) {
	m := jiraModel(t)
	m.tasks.active = true
	m.tasks.items = []model.Task{{Title: "First"}, {Title: "Second"}}
	m, _ = sequence(m, "ld")
	if m.tasks.selected != 1 || !m.tasks.details || m.jira.detail != nil || m.jira.active != 0 {
		t.Fatal("task navigation affected hidden Jira")
	}
}
func TestShortcutPromptsFitTerminal(t *testing.T) {
	m := ready(t)
	for _, size := range [][2]int{{80, 12}, {10, 2}, {1, 1}} {
		m.width, m.height = size[0], size[1]
		m.sequence = "at"
		view := m.View()
		if len(strings.Split(view.Content, "\n")) > m.height {
			t.Fatal("prompt exceeds height")
		}
		for _, line := range strings.Split(view.Content, "\n") {
			if ansi.StringWidth(line) > m.width {
				t.Fatal("prompt exceeds width")
			}
		}
		m.sequence = ""
		m.jiraAction = &jiraActionState{choices: []string{"QA", "In Progress", "Done"}, selected: 2}
		if len(strings.Split(m.View().Content, "\n")) > m.height {
			t.Fatal("picker exceeds height")
		}
		m.jiraAction = nil
	}
}
