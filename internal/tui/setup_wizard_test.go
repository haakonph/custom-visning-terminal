package tui

import (
	"errors"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	appconfig "github.com/haakonph/custom-visning-terminal/internal/config"
)

func TestWizardMaskedSecretsAndSequentialFields(t *testing.T) {
	isolateSetup(t)
	m := ready(t).startSetupWizard("jira", []string{"mattermost"})
	m.setup.wizard.input.SetValue("http://invalid.example")
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.setup.wizard.index != 0 || !strings.Contains(m.setup.wizard.notice, "HTTPS") {
		t.Fatal("invalid address advanced")
	}
	m.setup.wizard.input.SetValue("https://jira.example.test/context")
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.setup.wizard.index != 1 {
		t.Fatal("did not advance to token")
	}
	m.setup.wizard.input.SetValue("very-secret-token")
	if strings.Contains(m.View().Content, "very-secret-token") {
		t.Fatal("token exposed")
	}
	_, cmd := update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("missing validation command")
	}
	raw, err := os.ReadFile(appconfig.Path())
	if err != nil || strings.Contains(string(raw), "very-secret-token") {
		t.Fatal("secret persisted", err)
	}
}

func TestWizardFailureRetryAndStaleValidation(t *testing.T) {
	isolateSetup(t)
	m := ready(t).startSetupWizard("github", nil)
	w := m.setup.wizard
	w.busy = true
	next, _ := m.finishSetupValidation(setupValidated{wizard: w, err: errors.New("access denied")})
	m = next.(Model)
	if w.busy || w.valid || !strings.Contains(w.notice, "access denied") {
		t.Fatal("failure treated as success")
	}
	m = m.startSetupWizard("jira", nil)
	next, _ = m.finishSetupValidation(setupValidated{wizard: w})
	if next.(Model).setup.wizard.valid {
		t.Fatal("stale response accepted")
	}
}

func TestWizardContinuesOneIntegrationAtATime(t *testing.T) {
	isolateSetup(t)
	m := ready(t).startSetupWizard("github", []string{"jira", "mattermost"})
	m.setup.wizard.valid = true
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.setup.provider != "jira" || len(m.setup.wizard.queue) != 1 {
		t.Fatal("wrong next integration")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.setup.provider != "" {
		t.Fatal("cannot leave wizard")
	}
}

func TestWizardPastesAddressAndMaskedToken(t *testing.T) {
	isolateSetup(t)
	m := ready(t).startSetupWizard("jira", nil)
	defer m.cancel()
	m, _ = update(m, tea.PasteMsg{Content: "https://jira.example.test/context"})
	if got := m.setup.wizard.input.Value(); got != "https://jira.example.test/context" {
		t.Fatalf("pasted address lost: %q", got)
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.setup.wizard.index != 1 {
		t.Fatal("address did not advance to token")
	}
	const token = "pasted-secret-token"
	m, _ = update(m, tea.PasteMsg{Content: token})
	if m.setup.wizard.input.Value() != token {
		t.Fatal("pasted token lost")
	}
	if strings.Contains(m.View().Content, token) {
		t.Fatal("pasted token exposed")
	}
	raw, err := os.ReadFile(appconfig.Path())
	if err != nil || strings.Contains(string(raw), token) {
		t.Fatal("pasted token persisted", err)
	}
}

func TestWizardIgnoresPasteWhileValidatingOrComplete(t *testing.T) {
	for _, state := range []string{"busy", "valid"} {
		t.Run(state, func(t *testing.T) {
			isolateSetup(t)
			m := ready(t).startSetupWizard("github", nil)
			defer m.cancel()
			m.setup.wizard.input.SetValue("original")
			m.setup.wizard.busy = state == "busy"
			m.setup.wizard.valid = state == "valid"
			m, _ = update(m, tea.PasteMsg{Content: "unexpected"})
			if m.setup.wizard.input.Value() != "original" {
				t.Fatal("paste changed inactive field")
			}
		})
	}
}
