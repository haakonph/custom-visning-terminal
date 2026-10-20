package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	appconfig "github.com/haakonph/custom-visning-terminal/internal/config"
	mm "github.com/haakonph/custom-visning-terminal/internal/connectors/mattermost"
)

func TestStartupSetupOffersMissingIntegrationsAndCanBeSkipped(t *testing.T) {
	isolateSetup(t)
	m := ready(t)
	m.source = nil
	m = m.WithStartupSetup()
	if m.setup.provider != "startup" || len(m.setup.missing) != 3 || !strings.Contains(m.View().Content, "Vil du fullføre") {
		t.Fatalf("missing startup prompt: %+v", m.setup)
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.setup.provider != "" || !m.tasks.active || m.booting {
		t.Fatal("skip did not open usable local tasks")
	}
	// Skipping lasts for this run; a later startup offers setup again.
	m = m.WithStartupSetup()
	if m.setup.provider != "startup" {
		t.Fatal("missing setup suppressed on later startup")
	}
}

func TestStartupSetupSelectsHelpAndPreservesUnderlyingView(t *testing.T) {
	isolateSetup(t)
	m := ready(t)
	m.selected = 1
	m = m.WithStartupSetup()
	if len(m.setup.missing) != 2 || m.setup.missing[0] != "jira" {
		t.Fatalf("configured GitHub was offered: %+v", m.setup)
	}
	m, _ = update(m, key('j'))
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.setup.provider != "mattermost" || !strings.Contains(m.View().Content, "serveradresse") {
		t.Fatal("selected setup help did not open")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.setup.provider != "" || m.selected != 1 || m.tasks.active {
		t.Fatal("setup lost underlying GitHub selection")
	}
}

func TestStartupSetupCompleteAndCopilotOnlyDoNotPrompt(t *testing.T) {
	isolateSetup(t)
	m := ready(t)
	m.jira = &jiraState{}
	m.mmReader = &mattermostReader{}
	m = m.WithStartupSetup()
	if m.setup.provider != "" {
		t.Fatal("complete setup prompted")
	}
	m = ready(t)
	m.copilotActive = true
	m = m.WithStartupSetup()
	if m.setup.provider != "" {
		t.Fatal("Copilot-only startup prompted for unrelated integrations")
	}
}

func TestStartupSetupKeepsRequestedProviderAndProblem(t *testing.T) {
	isolateSetup(t)
	m := ready(t).WithSetupProblem("mattermost", "Kunne ikke åpne lokalt datalager")
	m = m.WithStartupSetup()
	if m.setup.missing[m.setup.selected] != "mattermost" {
		t.Fatal("startup lost requested provider")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.setup.provider != "mattermost" || !strings.Contains(m.setup.problem, "datalager") {
		t.Fatal("setup lost repair problem")
	}
}

func TestStartupSetupSelectionRemainsVisibleInSmallTerminal(t *testing.T) {
	isolateSetup(t)
	m := ready(t).WithStartupSetup()
	m, _ = update(m, tea.WindowSizeMsg{Width: 40, Height: 6})
	m, _ = update(m, key('j'))
	if !strings.Contains(m.View().Content, "> Mattermost") {
		t.Fatal(m.View().Content)
	}
	for _, line := range strings.Split(m.View().Content, "\n") {
		if ansi.StringWidth(line) > 40 {
			t.Fatal("startup prompt exceeds terminal width")
		}
	}
}

func isolateSetup(t *testing.T) {
	t.Helper()
	t.Setenv("CUSTOM_VISNING_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	for _, key := range []string{"MATTERMOST_SERVER", "MATTERMOST_USERNAME", "MATTERMOST_TOKEN"} {
		t.Setenv(key, "")
	}
}

func TestMissingProvidersOpenSetupAndPreserveCurrentSelection(t *testing.T) {
	for _, tc := range []struct{ keys, provider, instruction string }{
		{"vg", "github", "gh auth login"},
		{"vt", "jira", "JIRA_TOKEN"},
		{"vc", "mattermost", "serveradresse"},
		{"va", "copilot", "Copilot CLI"},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			isolateSetup(t)
			m := ready(t)
			m.selected = 1
			if tc.provider == "github" {
				m.source = nil
				m.tasks.active = true
			}
			before := m.baseProvider()
			m, cmd := sequence(m, tc.keys)
			if cmd != nil || m.setup.provider != tc.provider || m.baseProvider() != before || m.selected != 1 {
				t.Fatalf("wrong setup/state: %+v", m.setup)
			}
			body := m.View().Content
			if !strings.Contains(body, "OPPSETTSHJELP") || !strings.Contains(body, tc.instruction) {
				t.Fatal(body)
			}
			m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
			if m.setup.provider != "" || m.baseProvider() != before || m.selected != 1 {
				t.Fatal("return lost current view")
			}
		})
	}
}

func TestMattermostSetupInSettingsWorksWithoutRestart(t *testing.T) {
	isolateSetup(t)
	m := ready(t)
	if err := appconfig.Update(appconfig.Path(), map[string]any{"future": "keep"}); err != nil {
		t.Fatal(err)
	}
	m, _ = sequence(m, "vc")
	m, _ = sequence(m, "vs")
	if !m.settings.active || settingFields[m.settings.selected].path != "mattermost.server" {
		t.Fatal("settings did not select server")
	}
	m = editSetting(t, m, m.settings.selected, "https://chat.example.org/context")
	m = editSetting(t, m, m.settings.selected+1, "alice")
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.setup.provider != "mattermost" {
		t.Fatal("settings did not return to setup")
	}
	m, cmd := update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || m.mmReader == nil || !m.mmReader.active || m.setup.provider != "" {
		t.Fatal("setup required restart")
	}
	m, _ = update(m, mmResult{data: m.mmReader.data, err: mm.ErrSession})
	if m.mmReader.mode != "Password" {
		t.Fatal("login did not open")
	}
	m.mmReader.input.SetValue("password-not-for-config")
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.mmReader.password != "password-not-for-config" {
		t.Fatal("password not received")
	}
	raw, err := os.ReadFile(appconfig.Path())
	if err != nil || strings.Contains(string(raw), "password-not-for-config") || !strings.Contains(string(raw), "keep") {
		t.Fatalf("settings preservation/secrets: %s %v", raw, err)
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.mmReader.password != "" {
		t.Fatal("cancel did not clear password")
	}
}

func TestPartialAndInvalidMattermostSetupRemainsRepairable(t *testing.T) {
	for _, server := range []string{"", "https://chat.example.org", "http://insecure.example"} {
		t.Run(server, func(t *testing.T) {
			isolateSetup(t)
			m := ready(t)
			username := "alice"
			if server == "https://chat.example.org" {
				username = ""
			}
			if err := mm.SaveConfig(mm.Config{Server: server, Username: username}); err != nil {
				t.Fatal(err)
			}
			m, cmd := sequence(m, "vc")
			if cmd != nil || m.mmReader != nil || m.setup.provider != "mattermost" {
				t.Fatal("partial/invalid setup started reader")
			}
			m, _ = sequence(m, "vs")
			if !m.settings.active {
				t.Fatal("cannot repair partial setup")
			}
		})
	}
}

func TestSetupScrollingNavigationAndHelp(t *testing.T) {
	isolateSetup(t)
	m := ready(t)
	m, _ = sequence(m, "vh")
	if m.setup.provider != "github" {
		t.Fatal("no context help")
	}
	m, _ = sequence(m, "vc")
	m, _ = update(m, tea.WindowSizeMsg{Width: 24, Height: 6})
	first := m.View().Content
	for i := 0; i < 30; i++ {
		m, _ = update(m, key('j'))
	}
	if m.setup.offset == 0 || m.View().Content == first {
		t.Fatal("help cannot scroll")
	}
	for _, line := range strings.Split(m.View().Content, "\n") {
		if ansi.StringWidth(line) > 24 {
			t.Fatal("help exceeds width")
		}
	}
	if len(strings.Split(m.View().Content, "\n")) > 6 {
		t.Fatal("help exceeds height")
	}
	m, _ = update(m, key('?'))
	if !m.help {
		t.Fatal("keyboard help inaccessible")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.help || m.setup.provider != "mattermost" {
		t.Fatal("keyboard help did not return")
	}
	m, _ = sequence(m, "vo")
	if !m.tasks.active || m.setup.provider != "" {
		t.Fatal("cannot leave setup for tasks")
	}
}

func TestWebhookAndEditorActionsExplainMissingSetup(t *testing.T) {
	isolateSetup(t)
	m := ready(t)
	m, _ = sequence(m, "as")
	if m.setup.provider != "webhook" || !strings.Contains(m.View().Content, "MATTERMOST_WEBHOOK_URL") {
		t.Fatal("no webhook setup")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m, _ = sequence(m, "ee")
	if m.setup.provider != "editor" {
		t.Fatal("no editor setup")
	}
}

func TestFirstMattermostSetupPreservesPreviousProviderWithoutGitHub(t *testing.T) {
	isolateSetup(t)
	m := ready(t)
	m.source = nil
	m.tasks.active = true
	if err := mm.SaveConfig(mm.Config{Server: "https://chat.example.org", Username: "alice"}); err != nil {
		t.Fatal(err)
	}
	m, cmd := sequence(m, "vc")
	if cmd == nil || m.mmReader == nil || !m.mmReader.active || m.previousProvider != "task" {
		t.Fatal("first setup lost previous provider")
	}
	m, _ = sequence(m, "vr")
	if !m.tasks.active || m.mmReader.active {
		t.Fatal("could not return to tasks")
	}
}

func TestWebhookWizardActivatesSenderAndClearsSecret(t *testing.T) {
	isolateSetup(t)
	m := ready(t).WithSetup("webhook")
	next, _ := m.updateSetup("f")
	m = next.(Model)
	w := m.setup.wizard
	if w == nil || w.fields[0].kind != "secret" {
		t.Fatal("webhook field must be masked")
	}
	w.input.SetValue("https://chat.example/hooks/secret")
	next, cmd := m.updateSetupWizard(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	next, _ = m.finishSetupValidation(cmd().(setupValidated))
	m = next.(Model)
	if m.mattermost == nil || !w.valid {
		t.Fatal("webhook not activated")
	}
	if w.values[0] != "" || w.input.Value() != "" {
		t.Fatal("credential retained in form")
	}
	if strings.Contains(m.View().Content, "/hooks/secret") {
		t.Fatal("credential visible")
	}
}
