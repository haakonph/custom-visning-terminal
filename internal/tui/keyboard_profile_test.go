package tui

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	appconfig "github.com/haakonph/custom-visning-terminal/internal/config"
	"github.com/haakonph/custom-visning-terminal/internal/inputsource"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func withKeyboard(m Model, profile string) Model {
	m.keyboard.options = defaultKeyboardOptions()
	m.keyboard.options.Profile = profile
	m.setKeyboardProfile(profile)
	return m
}

func TestHomeKeyboardCommandsAndPermutation(t *testing.T) {
	for _, tc := range []struct{ profile, anchors, home string }{
		{"qwerty", "avde jkhl", "asdf jklø"},
		{"workman", "avde rjkhl", "ciae rhtns"},
	} {
		t.Run(tc.profile, func(t *testing.T) {
			m := withKeyboard(ready(t), tc.profile)
			if got := m.shortcut(tc.anchors); got != tc.home {
				t.Fatalf("resting positions: %q, want %q", got, tc.home)
			}
			seen := map[rune]bool{}
			for _, r := range "abcdefghijklmnopqrstuvwxyzæøå ;" {
				physical := translateKey(r, m.keyboard.encode)
				if seen[physical] || translateKey(physical, m.keyboard.decode) != r {
					t.Fatalf("collision or unreachable command at %q", r)
				}
				seen[physical] = true
			}
			m, _ = sequence(m, m.shortcut("j"))
			if m.selected != 1 {
				t.Fatal("home-row down did not navigate")
			}
			m, _ = sequence(m, " ")
			if len(m.chosen) != 1 {
				t.Fatal("thumb space did not select")
			}
			m, _ = sequence(m, m.shortcut("k"))
			m, _ = sequence(m, m.shortcut("l"))
			m, _ = sequence(m, m.shortcut("h"))
			if m.selected != 0 || m.active != 0 {
				t.Fatal("remaining right-hand navigation did not work")
			}
			m, _ = sequence(m, m.shortcut("v"))
			if m.sequence != "v" || !strings.Contains(m.View().Content, "Tastsekvens: "+m.shortcut("v")) {
				t.Fatal("profile prefix or displayed hint is wrong")
			}
			m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
			m, _ = sequence(m, m.shortcut("d"))
			if m.detail == nil {
				t.Fatal("home-row details shortcut did not reach internal dispatcher")
			}
		})
	}
}

func TestProfileTextInputAcrossProviders(t *testing.T) {
	const text = "ciae rhtns asdf jklø æøå"
	for _, tc := range []struct{ command, field string }{
		{"vg/", "github"}, {"vt/", "jira"}, {"va/", "copilot"},
		{"vc/", "mattermost"}, {"vo/", "task-search"}, {"voae", "task-edit"},
		{"vs", "settings"}, {"vcvk/", "channel-search"}, {"r", "dialog"},
	} {
		t.Run(tc.field, func(t *testing.T) {
			m := withKeyboard(unifiedModel(t), "workman")
			if tc.field == "dialog" {
				next, _ := m.showEntities("Velg", "related", m.allReferences(), nil)
				m = next.(Model)
				m.dialog.searching = true
			} else {
				m, _ = sequence(m, m.shortcut(tc.command))
			}
			var value func() string
			switch tc.field {
			case "github":
				value = func() string { return m.input.Value() }
			case "jira":
				value = func() string { return m.jira.input.Value() }
			case "copilot":
				value = func() string { return m.copilot.input.Value() }
			case "mattermost", "channel-search":
				value = func() string { return m.mmReader.input.Value() }
			case "task-search", "task-edit":
				m.tasks.input.SetValue("")
				value = func() string { return m.tasks.input.Value() }
			case "settings":
				m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
				m.settings.input.SetValue("")
				value = func() string { return m.settings.input.Value() }
			case "dialog":
				value = func() string { return m.dialog.query }
			}
			if m.keyboardCommandMode() {
				t.Fatal("text field classified as command mode")
			}
			m, _ = sequence(m, text)
			if got := value(); got != text || m.sequence != "" {
				t.Fatalf("text was remapped: %q", got)
			}
		})
	}
}

func TestInputSourceLearningAndSwitching(t *testing.T) {
	t.Setenv("CUSTOM_VISNING_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	if err := appconfig.Update(appconfig.Path(), map[string]any{"keyboard": map[string]any{"future": "keep"}, "other": "keep"}); err != nil {
		t.Fatal(err)
	}
	m := ready(t)
	m.keyboard.currentSource = nil
	qwerty := keyboardSourceMsg{source: inputsource.Source{ID: "test.norsk", Name: "Norsk"}}
	workman := keyboardSourceMsg{source: inputsource.Source{ID: "test.utf8", Name: "UTF8"}}
	m, _ = update(m, qwerty)
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyF2})
	if m.keyboard.profile != "qwerty" || m.keyboard.options.Profile != "auto" {
		t.Fatal("F2 did not learn current source")
	}
	m, _ = update(m, workman)
	if m.keyboard.profile != "standard" {
		t.Fatal("unknown source reused another keyboard's shortcuts")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyF2})
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyF2})
	if m.keyboard.profile != "workman" {
		t.Fatal("external profile not learned")
	}
	m, _ = sequence(m, m.shortcut("a"))
	if m.sequence != "a" {
		t.Fatal("action prefix not started")
	}
	m, _ = update(m, qwerty)
	if m.keyboard.profile != "qwerty" || m.sequence != "" || len(m.sequenceRefs) != 0 {
		t.Fatal("source switch did not cancel pending action")
	}
	m.loadKeyboard()
	m, _ = update(m, workman)
	if m.keyboard.profile != "workman" {
		t.Fatal("saved source mapping not reloaded")
	}
	doc, err := settingsDocument()
	if err != nil || settingValue(doc, "keyboard.future") != "keep" || doc["other"] != "keep" {
		t.Fatal("learning overwrote unrelated settings", err)
	}
	m, _ = sequence(m, "/literal")
	m, _ = update(m, qwerty)
	m, _ = sequence(m, " æøå")
	if m.input.Value() != "literal æøå" {
		t.Fatal("source switch rewrote text input", m.input.Value())
	}
	m, _ = update(m, keyboardSourceMsg{err: errors.New("unavailable")})
	if m.keyboard.source.ID != "" || m.keyboard.sourceError == "" || m.input.Value() != "literal æøå" {
		t.Fatal("failed source read retained stale identity or changed text")
	}
}

func TestProfileSwitchSaveFailureAndManualOverride(t *testing.T) {
	t.Setenv("CUSTOM_VISNING_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	m := ready(t)
	m.keyboard.options.Profile = "qwerty"
	m.setKeyboardProfile("qwerty")
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyF2})
	if m.keyboard.profile != "workman" || m.keyboard.options.Profile != "workman" {
		t.Fatal("manual profile did not persist")
	}
	m, _ = update(m, keyboardSourceMsg{source: inputsource.Source{ID: "another", Name: "Other"}})
	if m.keyboard.profile != "workman" {
		t.Fatal("automatic detection overrode manual profile")
	}
	t.Setenv("CUSTOM_VISNING_CONFIG", filepath.Join(appconfig.Path(), "impossible"))
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyF2})
	if m.keyboard.profile != "workman" || !strings.Contains(m.notice, "Kunne ikke lagre") {
		t.Fatal("failed save changed profile")
	}
}

func TestInputSourceCheckedBeforeDecodingKey(t *testing.T) {
	m := ready(t)
	m.keyboard.options.SourceProfiles = map[string]string{"laptop": "qwerty", "external": "workman"}
	current := inputsource.Source{ID: "laptop"}
	m.keyboard.currentSource = func() (inputsource.Source, error) { return current, nil }
	m, _ = update(m, key('a'))
	if m.keyboard.profile != "qwerty" || m.sequence != "a" {
		t.Fatal("initial source was not read before handling action prefix")
	}
	current = inputsource.Source{ID: "external"}
	m, cmd := update(m, key('h'))
	if m.keyboard.profile != "workman" || m.sequence != "" || m.selected != 1 || cmd != nil {
		t.Fatal("first external key used stale profile or continued the old action")
	}
}

func TestKeyboardConfigurationAndHints(t *testing.T) {
	for _, home := range []string{"", "asdfjklø", "asdf  jklø", "Asdf jklø", "aaaa jklø", "asdf jkl1", "asdf\tjklø"} {
		if _, err := homeKeyMap(home); err == nil {
			t.Fatalf("accepted malformed resting keys: %q", home)
		}
	}
	for _, home := range []string{"asdf jklø", "asdf jkl;", "ciae rhtns"} {
		if _, err := homeKeyMap(home); err != nil {
			t.Fatal(home, err)
		}
	}
	m := withKeyboard(ready(t), "workman")
	if got := m.keyHints("j/k velg · d detaljer · Space marker · Shift+↑/↓ utvid · Ctrl+C avslutt · ?/Esc tilbake"); got != "h/t velg · a detaljer · Space marker · Shift+↑/↓ utvid · Ctrl+C avslutt · ?/Esc tilbake" {
		t.Fatal(got)
	}
	for _, msg := range []tea.KeyPressMsg{{Code: 'c', Mod: tea.ModCtrl}, {Code: 'a', Mod: tea.ModAlt}, {Code: tea.KeyDown, Mod: tea.ModShift}} {
		if got := m.profileKey(msg); got != msg {
			t.Fatal("changed modified shortcut", got)
		}
	}
	for _, field := range settingFields {
		if field.kind == "keyboard-sources" {
			if _, err := parseSetting(field, `{"id":"workman"}`); err != nil {
				t.Fatal(err)
			}
			for _, invalid := range []string{`null`, `[]`, `{"":"qwerty"}`, `{"id":"unknown"}`} {
				if _, err := parseSetting(field, invalid); err == nil {
					t.Fatal("accepted invalid source mapping", invalid)
				}
			}
		}
	}
}

func TestProfilePreservesTimeInputAndDeleteConfirmation(t *testing.T) {
	m := withKeyboard(ready(t), "workman")
	m.snoozes.input = true
	if got := m.profileKey(key('h')); got.String() != "h" {
		t.Fatal("hour unit was decoded as home-row navigation")
	}
	m.snoozes.input = false
	m.local = &localState{removePending: []model.PullRequest{{Number: 1}}}
	m, cmd := update(m, key('n'))
	if len(m.local.removePending) != 0 || cmd != nil {
		t.Fatal("literal n did not cancel worktree deletion")
	}
}
