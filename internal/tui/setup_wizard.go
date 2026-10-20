package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/github"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/jira"
	mm "github.com/haakonph/custom-visning-terminal/internal/connectors/mattermost"
)

type setupWizard struct {
	fields          []settingField
	values          []string
	index           int
	input           textinput.Model
	busy, valid     bool
	loginMattermost bool
	notice          string
	queue           []string
}
type setupValidated struct {
	wizard  *setupWizard
	github  *github.Client
	jira    *jira.Client
	webhook *mm.Client
	err     error
}

func (m Model) startSetupWizard(provider string, queue []string) Model {
	m = m.WithSetup(provider)
	w := &setupWizard{queue: append([]string(nil), queue...)}
	switch provider {
	case "github":
		w.fields = []settingField{{"GitHub · PAT (tomt bruker gh / miljø)", "GITHUB_TOKEN", "secret"}}
	case "jira":
		w.fields = []settingField{{"Jira · serveradresse", "jira.base_url", "url"}, {"Jira · PAT", "JIRA_TOKEN", "secret"}}
	case "webhook":
		w.fields = []settingField{{"Mattermost · incoming-webhook (HTTPS)", "MATTERMOST_WEBHOOK_URL", "secret"}}
	case "mattermost":
		w.fields = []settingField{{"Mattermost · serveradresse", "mattermost.server", "url"}, {"Mattermost · brukernavn", "mattermost.username", "string"}}
	default:
		return m
	}
	doc, err := settingsDocument()
	if err != nil {
		w.notice = "Kunne ikke lese oppsettet. Rett config-filen før lagring."
	}
	for _, f := range w.fields {
		value, _ := settingValue(doc, f.path).(string)
		env := map[string]string{"jira.base_url": "JIRA_BASE_URL", "mattermost.server": "MATTERMOST_SERVER", "mattermost.username": "MATTERMOST_USERNAME"}[f.path]
		if f.kind == "secret" {
			env = f.path
		}
		if v := os.Getenv(env); v != "" {
			value = v
		}
		w.values = append(w.values, value)
	}
	w.input = textinput.New()
	w.input.CharLimit = 4096
	w.input.SetWidth(max(1, m.width-4))
	w.loadField()
	m.setup.wizard = w
	return m
}
func (w *setupWizard) loadField() {
	w.input.EchoMode = textinput.EchoNormal
	if w.fields[w.index].kind == "secret" {
		w.input.EchoMode = textinput.EchoPassword
	}
	w.input.SetValue(w.values[w.index])
	w.input.Focus()
}
func (m Model) wizardView() tea.View {
	w := m.setup.wizard
	lines := []string{accent.Render(" OPPSETT · " + m.setup.provider), fmt.Sprintf("Felt %d av %d · %s", w.index+1, len(w.fields), w.fields[w.index].label), w.input.View(), "Token brukes bare i denne kjøringen; offentlige felter lagres i config.", clean(w.notice), "Enter neste / valider · Shift+Tab forrige · Esc tilbake"}
	if w.busy {
		lines[2] = "Validerer tilkobling …"
	}
	if w.valid {
		lines[2] = "Oppsett klart. Enter fortsetter."
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], max(1, m.width), "…")
	}
	lines = lines[:min(len(lines), max(1, m.height))]
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	return v
}
func (m Model) updateSetupWizard(msg tea.Msg) (tea.Model, tea.Cmd) {
	w := m.setup.wizard
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc":
			m.setup = setupState{}
			return m, nil
		case "shift+tab":
			if !w.busy {
				w.values[w.index] = w.input.Value()
				w.index = max(0, w.index-1)
				w.valid = false
				w.loadField()
			}
			return m, nil
		case "enter":
			if w.busy {
				return m, nil
			}
			if w.valid {
				if len(w.queue) > 0 {
					m = m.startSetupWizard(w.queue[0], w.queue[1:])
					m.setup.wizard.loginMattermost = w.loginMattermost
					return m, m.setup.wizard.input.Focus()
				}
				m.setup = setupState{}
				if w.loginMattermost && m.mmReader != nil {
					next, _ := m.viewProvider("c")
					m = next.(Model)
					return m, m.mmStart()
				}
				return m, nil
			}
			value := strings.TrimSpace(w.input.Value())
			f := w.fields[w.index]
			if value == "" && !(m.setup.provider == "github") {
				w.notice = "Fyll ut " + f.label
				return m, nil
			}
			if f.kind != "secret" {
				if err := saveSetting(f, value); err != nil {
					w.notice = "Kunne ikke lagre: " + err.Error()
					return m, nil
				}
			}
			w.values[w.index] = value
			if w.index < len(w.fields)-1 {
				w.index++
				w.notice = ""
				w.loadField()
				return m, w.input.Focus()
			}
			w.busy = true
			w.notice = ""
			provider := m.setup.provider
			values := append([]string(nil), w.values...)
			parent := m.ctx
			return m, func() tea.Msg { return validateSetup(parent, w, provider, values) }
		}
	}
	if w.busy || w.valid {
		return m, nil
	}
	var cmd tea.Cmd
	w.input, cmd = w.input.Update(msg)
	return m, cmd
}
func validateSetup(parent context.Context, w *setupWizard, provider string, v []string) setupValidated {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	r := setupValidated{wizard: w}
	switch provider {
	case "github":
		token := v[0]
		if token == "" {
			token, r.err = github.Token(ctx)
		}
		if r.err == nil {
			r.github, r.err = github.New(token)
		}
		if r.err == nil {
			_, r.err = r.github.User(ctx)
		}
	case "jira":
		r.jira, r.err = jira.New(v[0], v[1])
		if r.err == nil {
			_, r.err = r.jira.Search(ctx, "assignee = currentUser() AND resolution = Unresolved")
		}
	case "webhook":
		r.webhook, r.err = mm.New(v[0])
	case "mattermost":
		_, r.err = mm.NewAPI(v[0])
	}
	return r
}
func (m Model) finishSetupValidation(r setupValidated) (tea.Model, tea.Cmd) {
	w := m.setup.wizard
	if w == nil || w != r.wizard {
		return m, nil
	}
	w.busy = false
	if r.err != nil {
		w.notice = "Validering feilet: " + r.err.Error() + " · Shift+Tab retter tidligere felt, Enter prøver igjen."
		return m, nil
	}
	if r.webhook != nil {
		m = m.WithMattermost(r.webhook)
	}
	if r.github != nil {
		m.source = r.github
	}
	if r.jira != nil {
		m = m.WithJira(r.jira, false)
	}
	if m.setup.provider == "mattermost" {
		var err error
		m, err = m.withMattermostSettings(mm.Config{Server: w.values[0], Username: w.values[1]}, os.Getenv("MATTERMOST_TOKEN"))
		if err != nil {
			w.notice = "Kunne ikke klargjøre lokalt Mattermost-oppsett."
			return m, nil
		}
	}
	var cmd tea.Cmd
	if r.github != nil {
		cmd = m.bootstrap()
	}
	if r.jira != nil {
		cmd = m.refreshJira()
	}
	w.valid = true
	w.notice = "Validert mot API-et."
	if m.setup.provider == "webhook" {
		w.notice = "HTTPS-adressen er kontrollert lokalt; ingen testmelding er sendt. Klar for as. Gjelder til appen avsluttes."
	}
	if m.setup.provider == "mattermost" {
		w.loginMattermost = true
		w.notice = "Oppsett lagret. Fortsett for å logge inn med passord og eventuell TOTP-kode."
	}
	// Remove credentials from the form once the authenticated clients own them.
	for i, f := range w.fields {
		if f.kind == "secret" {
			w.values[i] = ""
		}
	}
	w.input.SetValue("")
	return m, cmd
}
