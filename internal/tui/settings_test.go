package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	appconfig "github.com/haakonph/custom-visning-terminal/internal/config"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/copilot"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/jira"
	"github.com/haakonph/custom-visning-terminal/internal/local"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func editSetting(t *testing.T, m Model, index int, value string) Model {
	t.Helper()
	m.settings.selected = index
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.settings.editing {
		t.Fatal(m.settings.notice)
	}
	m.settings.input.SetValue(value)
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	return m
}
func TestSettingsSaveReloadAndReset(t *testing.T) {
	for _, jiraActive := range []bool{false, true} {
		m := jiraModel(t)
		defer m.cancel()
		m.jiraActive = jiraActive
		query := jira.Query{Name: "Mine", JQL: "assignee = currentUser()"}
		if err := jira.SaveConfig(jira.ConfigPath(), jira.Config{Queries: []jira.Query{query}}); err != nil {
			t.Fatal(err)
		}
		m, _ = update(m, key(','))
		m = editSetting(t, m, 0, `KAKE-[0-9]{3,5}`)
		if !m.settings.active || m.settings.editing {
			t.Fatal(m.settings.notice)
		}
		config, err := jira.LoadConfig(jira.ConfigPath())
		if err != nil || config.Pattern != `KAKE-[0-9]{3,5}` || len(config.Queries) != 1 {
			t.Fatalf("config not preserved: %+v %v", config, err)
		}
		reloaded := New(context.Background(), nil, nil).WithJiraPattern(nil)
		defer reloaded.cancel()
		p := model.PullRequest{Title: "KAKE-123 ABC-12"}
		if got := p.JiraKeys(reloaded.jiraPattern); len(got) != 1 || got[0] != "KAKE-123" {
			t.Fatal(got)
		}
		m = editSetting(t, m, 0, "")
		if m.jiraPattern != nil {
			t.Fatal("reset failed")
		}
		m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
		if m.settings.active || m.jiraActive != jiraActive {
			t.Fatal("return changed provider")
		}
	}
}
func TestSettingsInvalidCancelAndWriteFailure(t *testing.T) {
	m := jiraModel(t)
	defer m.cancel()
	m, _ = update(m, key(','))
	m = editSetting(t, m, 0, "[")
	if !m.settings.editing || m.settings.notice == "" || m.jiraPattern != nil {
		t.Fatal("invalid pattern accepted")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.settings.editing || !m.settings.active {
		t.Fatal("cancel edit failed")
	}
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	t.Setenv("CUSTOM_VISNING_CONFIG", filepath.Join(blocker, "config.json"))
	m.settings.input.SetValue(`KAKE-[0-9]{3,5}`)
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.settings.editing || m.settings.notice == "" || m.jiraPattern != nil {
		t.Fatal("write failure changed active pattern")
	}
}
func TestSettingsUseOneFileAndPreserveOtherSections(t *testing.T) {
	m := jiraModel(t)
	defer m.cancel()
	if err := appconfig.Update(appconfig.Path(), map[string]any{"future": "keep"}); err != nil {
		t.Fatal(err)
	}
	m, _ = update(m, key(','))
	for _, tc := range []struct {
		index int
		value string
	}{{0, `KAKE-[0-9]{3,5}`}, {1, "https://jira.example"}, {4, `["/workspace/repo"]`}, {5, "vscode"}, {14, "4319"}, {15, "false"}} {
		m = editSetting(t, m, tc.index, tc.value)
		if m.settings.editing {
			t.Fatal(m.settings.notice)
		}
	}
	localConfig, err := local.LoadConfig(appconfig.Path())
	if err != nil || localConfig.Editors.Default != "vscode" || len(localConfig.RepositoryPaths) != 1 {
		t.Fatal(localConfig, err)
	}
	options, err := copilot.OptionsFromEnv()
	if err != nil || options.Port != 4319 || options.Enabled {
		t.Fatal(options, err)
	}
	jiraConfig, err := jira.LoadConfig(appconfig.Path())
	if err != nil || jiraConfig.BaseURL != "https://jira.example" || jiraConfig.Pattern == "" {
		t.Fatal(jiraConfig, err)
	}
	doc, _ := appconfig.Read(appconfig.Path())
	if string(doc["future"]) != `"keep"` {
		t.Fatal("unknown settings lost")
	}
	t.Setenv("COPILOT_OTLP_PORT", "4320")
	options, err = copilot.OptionsFromEnv()
	if err != nil || options.Port != 4320 {
		t.Fatal("environment override failed")
	}
}
func TestSettingsValidation(t *testing.T) {
	for _, tc := range []struct {
		index int
		value string
	}{{0, "["}, {1, "http://jira.example"}, {4, `["relative"]`}, {5, "bad"}, {7, `[1]`}, {12, "relative"}, {14, "70000"}, {15, "yes"}, {3, `[{"name":"incomplete"}]`}} {
		if _, err := parseSetting(settingFields[tc.index], tc.value); err == nil {
			t.Fatalf("accepted %s=%s", settingFields[tc.index].path, tc.value)
		}
	}
}
func TestSettingsDoesNotOverwriteMalformedFile(t *testing.T) {
	t.Setenv("CUSTOM_VISNING_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	original := []byte(`{"jira":`)
	if err := os.WriteFile(appconfig.Path(), original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := saveSetting(settingFields[0], "KAKE-[0-9]{3,5}"); err == nil {
		t.Fatal("accepted malformed config")
	}
	got, _ := os.ReadFile(appconfig.Path())
	if string(got) != string(original) {
		t.Fatal("damaged file overwritten")
	}
}
func TestSettingsFromCopilotAndTypingComma(t *testing.T) {
	m := jiraModel(t)
	defer m.cancel()
	m.copilot = &copilotState{}
	m.copilotActive = true
	m, _ = update(m, key(','))
	if !m.settings.active {
		t.Fatal("settings unavailable in Copilot")
	}
	m.settings.active = false
	m.copilotActive = false
	m.jiraActive = false
	m.filtering = true
	m.input.Focus()
	m, _ = update(m, key(','))
	if m.settings.active || !strings.Contains(m.input.Value(), ",") {
		t.Fatal("intercepted filter input")
	}
}

func TestSettingsTokenHelpNavigation(t *testing.T) {
	m := jiraModel(t)
	defer m.cancel()
	m, _ = update(m, key(','))
	m.width, m.height = 60, 12
	m.settings.selected = 1
	m, _ = update(m, key('?'))
	if !m.settings.help || m.settings.editing {
		t.Fatal("token help did not open")
	}
	m, _ = update(m, key('j'))
	if m.settings.helpOffset != 1 || m.settings.selected != 1 {
		t.Fatal("help scrolling changed selected setting")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.settings.editing {
		t.Fatal("help allowed editing")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.settings.help || !m.settings.active {
		t.Fatal("escape did not return to settings")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = update(m, key('?'))
	if m.settings.help || !strings.Contains(m.settings.input.Value(), "?") {
		t.Fatal("help intercepted editing input")
	}
}
