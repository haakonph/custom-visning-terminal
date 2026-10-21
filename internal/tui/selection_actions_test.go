package tui

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/jira"
	"github.com/haakonph/custom-visning-terminal/internal/local"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/snooze"
)

// Exercise the same key messages as the terminal, never assign chosen directly.
// Leave an unselected third PR highlighted so actions cannot pass by accidentally
// using the cursor instead of the Space selection.
func spaceActionModel(t *testing.T, scenario string) (Model, []model.PullRequest) {
	t.Helper()
	m := snoozeReady(t)
	rows := append([]model.PullRequest(nil), m.filtered()...)
	rows[0].Title = "ABC-1 ABC-9 first"
	rows[1].Title = "ABC-2 ABC-9 second"
	rows = append(rows, model.PullRequest{Repository: "acme/api", Number: 3, URL: "https://github.com/acme/api/pull/3", Title: "ABC-3 unselected"})
	m.snapshot.Datasets[model.Open].PRs = rows
	m = press(m, " j j") // Space on rows 1 and 2; cursor on row 3.
	want := rows[:2]
	switch scenario {
	case "across views and filter":
		// Remove the second selection and reselect it from another view.
		m = press(m, "k ")
		m.snapshot.Datasets[model.Reviews].PRs = []model.PullRequest{rows[1], rows[2]}
		m = press(m, "ll ")
		m = press(m, "/no matches")
		m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
		if len(m.filtered()) != 0 {
			t.Fatal("filter did not hide selected rows")
		}
	case "deselected":
		m = press(m, "k j") // Deselect row 2, return cursor to row 3.
		want = rows[:1]
	case "no selection":
		m = press(m, "k k jj") // Toggle both selections off, highlight row 3.
		want = rows[2:]
	}
	expectedSelected := len(want)
	if scenario == "no selection" {
		expectedSelected = 0
	}
	if len(m.chosen) != expectedSelected {
		t.Fatalf("Space selected %d PRs, want %d", len(m.chosen), expectedSelected)
	}
	return m, want
}

func assertActionURLs(t *testing.T, got []string, want []model.PullRequest) {
	t.Helper()
	expected := make([]string, len(want))
	for i, p := range want {
		expected[i] = p.URL
	}
	slices.Sort(got)
	slices.Sort(expected)
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("action URLs = %v, want %v", got, expected)
	}
}

type selectionAction struct {
	name string
	run  func(*testing.T, Model, []model.PullRequest)
}

func TestSpaceSelectionActions(t *testing.T) {
	actions := []selectionAction{
		{"browser en", func(t *testing.T, m Model, want []model.PullRequest) {
			var opened []string
			m.open = func(_ context.Context, url string) error { opened = append(opened, url); return nil }
			_, cmd := sequence(m, "en")
			if cmd == nil {
				t.Fatal("Enter did not open browser")
			}
			if msg := cmd().(openMsg); msg.err != nil {
				t.Fatal(msg.err)
			}
			assertActionURLs(t, opened, want)
		}},
		{"snooze z", func(t *testing.T, m Model, want []model.PullRequest) {
			m = press(m, "z2h")
			saved, err := m.snoozes.store.Load()
			if err != nil {
				t.Fatal(err)
			}
			var urls []string
			for _, entry := range saved {
				urls = append(urls, entry.URL)
				if !entry.Until.Equal(m.snoozes.entries[entry.ID].Until) {
					t.Fatal("deadline not persisted")
				}
			}
			assertActionURLs(t, urls, want)
			if m.snoozes.input || len(m.chosen) != 0 {
				t.Fatal("snooze did not finish and clear selection")
			}
			m.input.SetValue("")
			for _, p := range want {
				if !m.snoozes.hidden(snooze.Identity("github", p.URL)) {
					t.Fatalf("%s still visible", p.URL)
				}
			}
		}},
	}
	for _, actionKey := range []rune{'t', 'a'} {
		actions = append(actions, selectionAction{
			fmt.Sprintf("task %c", actionKey), func(t *testing.T, m Model, want []model.PullRequest) {
				existing := model.TaskReference{Provider: "jira", URL: "https://jira.example/browse/OLD-1", Title: "Existing"}
				m.tasks.items = []model.Task{{Title: "Existing task", References: []model.TaskReference{existing}}}
				if actionKey == 't' {
					m, _ = sequence(m, "voan")
					m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
					m, _ = sequence(m, "vrto")
					for i, ref := range m.dialog.filtered() {
						if ref.Key == m.tasks.items[1].ID {
							m.dialog.selected = i
						}
					}
					m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
				} else {
					m, _ = sequence(m, "to")
					m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
				}
				taskIndex := 0
				if actionKey == 't' {
					taskIndex = 1
				}
				if len(m.tasks.items) != taskIndex+1 {
					t.Fatal("wrong task created/selected")
				}
				saved := newTasks()
				if saved.loadFailed || len(saved.items) != len(m.tasks.items) {
					t.Fatal("tasks not saved")
				}
				var urls []string
				for _, ref := range saved.items[taskIndex].References {
					if ref.Provider == "github" {
						urls = append(urls, ref.URL)
					}
				}
				assertActionURLs(t, urls, want)
				if saved.items[0].References[0] != existing {
					t.Fatal("existing task reference lost")
				}
			},
		})
	}
	for _, keys := range []string{"ee", "ev\n", "evj\n", "evjj\n"} {
		actions = append(actions, selectionAction{
			"editor " + keys, func(t *testing.T, m Model, want []model.PullRequest) {
				manager, err := local.New(t.TempDir(), local.Defaults())
				if err != nil {
					t.Fatal(err)
				}
				// Validate scheduling without launching a real editor or fetching GitHub.
				binary, err := os.Executable()
				if err != nil {
					t.Fatal(err)
				}
				editor := local.Editor{Command: binary}
				m = m.WithLocal(manager, local.Editors{Default: "intellij", IntelliJ: editor, VSCode: editor, Custom: editor})
				var cmd tea.Cmd
				for _, k := range keys {
					if k == '\n' {
						m, cmd = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
					} else {
						m, cmd = update(m, key(k))
					}
				}
				if cmd == nil || m.local.running != len(want) || m.local.choosing {
					t.Fatal("selected editor operations not scheduled")
				}
				var urls []string
				for url := range m.local.preparing {
					urls = append(urls, url)
				}
				assertActionURLs(t, urls, want)
				expectedEditor := map[string]string{"ee": "intellij", "ev\n": "intellij", "evj\n": "vscode", "evjj\n": "custom"}[keys]
				if m.local.editor != expectedEditor {
					t.Fatalf("editor = %q, want %q", m.local.editor, expectedEditor)
				}
			},
		})
	}
	actions = append(actions, selectionAction{
		"Mattermost m", func(t *testing.T, m Model, want []model.PullRequest) {
			t.Setenv("PATH", t.TempDir()) // No interactive Vim process in tests.
			sender := &fakeMessageSender{}
			m = m.WithMattermost(sender)
			m, _ = sequence(m, "as")
			if !m.mattermost.active {
				t.Fatal("Mattermost draft not opened")
			}
			expected := make([]string, len(want))
			for i, p := range want {
				expected[i] = fmt.Sprintf("[%s](%s)", p.Title, p.URL)
			}
			if m.mattermost.text != strings.Join(expected, "\n") {
				t.Fatalf("draft = %q, want %v", m.mattermost.text, expected)
			}
			draft := m.mattermost.text
			m, cmd := sequence(m, "as")
			if cmd == nil {
				t.Fatal("send command missing")
			}
			m, _ = update(m, cmd())
			if sender.calls != 1 || sender.text != draft || m.mattermost.active {
				t.Fatal("selected PR draft not sent")
			}
		},
	}, selectionAction{
		"related Jira rt", func(t *testing.T, m Model, want []model.PullRequest) {
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			client, err := jira.New("https://jira.example", "test")
			if err != nil {
				t.Fatal(err)
			}
			m.jira = &jiraState{client: client}
			m, _ = update(m, key('r'))
			m, cmd := update(m, key('t'))
			if len(want) > 0 && m.dialog != nil {
				if m.dialog.mode != "related" {
					t.Fatal("not a related picker")
				}
				var got []string
				for _, r := range m.dialog.rows {
					got = append(got, r.Key)
				}
				var expected []string
				seen := map[string]bool{}
				for _, p := range want {
					for _, key := range p.JiraKeys() {
						if !seen[key] {
							seen[key] = true
							expected = append(expected, key)
						}
					}
				}
				slices.Sort(got)
				slices.Sort(expected)
				if !reflect.DeepEqual(got, expected) {
					t.Fatalf("targets %v want %v", got, expected)
				}
			} else if cmd == nil || !m.jiraActive {
				t.Fatal("single Jira target not opened")
			}

		},
	})
	for _, action := range actions {
		t.Run(action.name, func(t *testing.T) {
			for _, scenario := range []string{"multiple", "across views and filter", "deselected", "no selection"} {
				t.Run(scenario, func(t *testing.T) {
					m, want := spaceActionModel(t, scenario)
					action.run(t, m, want)
				})
			}
		})
	}
}

func TestSpaceSelectionSnoozeUnitsAndCancel(t *testing.T) {
	for _, unit := range []string{"h", "d", "D"} {
		t.Run(unit, func(t *testing.T) {
			m, want := spaceActionModel(t, "multiple")
			// Cancel after entering a duration: no selection or stored deadline changes.
			m = press(m, "z12")
			m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
			if m.snoozes.input || len(m.chosen) != 2 || len(m.snoozes.entries) != 0 {
				t.Fatal("cancel changed selection or snoozes")
			}
			now := m.snoozes.now()
			m.snoozes.now = func() time.Time { return now }
			expected, err := snooze.Deadline(now.In(time.Local), "2", unit)
			if err != nil {
				t.Fatal(err)
			}
			keyUnit := unit
			if keyUnit == "D" {
				keyUnit = "m"
			}
			m = press(m, "z2"+keyUnit)
			saved, err := m.snoozes.store.Load()
			if err != nil {
				t.Fatal(err)
			}
			var urls []string
			for _, entry := range saved {
				urls = append(urls, entry.URL)
				if !entry.Until.Equal(expected) {
					t.Fatalf("deadline = %s, want %s", entry.Until, expected)
				}
			}
			assertActionURLs(t, urls, want)
		})
	}
}
