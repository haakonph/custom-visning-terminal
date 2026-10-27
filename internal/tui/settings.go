package tui

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	appconfig "github.com/haakonph/custom-visning-terminal/internal/config"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/copilot"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/jira"
	"github.com/haakonph/custom-visning-terminal/internal/local"
)

type settingField struct{ label, path, kind string }

var settingFields = []settingField{
	{"Jira · nøkkelmønster", "jira.pattern", "regex"},
	{"Jira · serveradresse", "jira.base_url", "url"},
	{"Jira · standardprosjekt", "jira.project", "string"},
	{"Jira · lagrede søk (JSON)", "jira.queries", "queries"},
	{"Lokalt · repository paths (JSON)", "repository_paths", "paths"},
	{"Editor · standard", "editors.default", "editor"},
	{"IntelliJ · kommando", "editors.intellij.command", "string"},
	{"IntelliJ · argumenter (JSON)", "editors.intellij.args", "list"},
	{"VS Code · kommando", "editors.vscode.command", "string"},
	{"VS Code · argumenter (JSON)", "editors.vscode.args", "list"},
	{"Egen editor · kommando", "editors.custom.command", "string"},
	{"Egen editor · argumenter (JSON)", "editors.custom.args", "list"},
	{"Copilot · sesjonsmappe", "copilot.session_root", "path"},
	{"Copilot · datalager", "copilot.store_path", "path"},
	{"Copilot · OTLP-port", "copilot.port", "port"},
	{"Copilot · innsamling", "copilot.enabled", "bool"},
	{"Copilot · lagre innhold", "copilot.capture_content", "bool"},
	{"Copilot · vis innhold", "copilot.show_content", "bool"},
	{"Copilot · Jira-mønster", "copilot.jira_pattern", "regex"},
	{"Copilot · IntelliJ-launcher", "copilot.intellij_command", "string"},
	{"Mattermost · serveradresse", "mattermost.server", "url"},
	{"Mattermost · brukernavn", "mattermost.username", "string"},
	{"Lokalt · klargjør main automatisk", "workspace.warm", "bool"},
	{"Lokalt · installer avhengigheter", "workspace.install", "bool"},
	{"Lokalt · hovedbranch", "workspace.main_branch", "string"},
	{"Lokalt · Maven-flagg (JSON)", "workspace.maven_flags", "list"},
	{"IntelliJ headless · kommando", "workspace.indexer.command", "string"},
	{"IntelliJ headless · argumenter (JSON)", "workspace.indexer.args", "list"},
	{"GitHub · skjulte organisasjonsprefikser (JSON)", "github.hidden_prefixes", "list"},
	{"Oppdatering · automatisk", "refresh.enabled", "bool"},
	{"Oppdatering · aktiv visning", "refresh.active", "duration"},
	{"Oppdatering · bakgrunn", "refresh.background", "duration"},
	{"Oppdatering · PR-detaljer", "refresh.details", "duration"},
	{"Oppdatering · lokal aktiv", "refresh.local_active", "duration"},
	{"Oppdatering · lokal bakgrunn", "refresh.local_background", "duration"},
	{"Varsler · CI ferdig", "notifications.ci", "bool"},
	{"Varsler · Copilot-review", "notifications.copilot", "bool"},
}

type settingsState struct {
	request *settingsRequest
	busy    bool

	active, editing bool
	editorPicker    bool
	chooseCustom    bool
	editorChoice    int
	help            bool
	helpOffset      int
	selected        int
	input           textinput.Model
	notice          string
	values          []string
}

func (m *Model) loadJiraPattern() {
	config, err := jira.LoadConfig(jira.ConfigPath())
	if err == nil && config.Pattern != "" {
		m.jiraPattern, err = regexp.Compile(config.Pattern)
	}
	if err != nil {
		m.notice = "Kunne ikke lese konfigurasjonen: " + err.Error()
	}
}

func settingsDocument() (map[string]any, error) {
	doc, err := appconfig.Read(appconfig.Path())
	if err != nil {
		return nil, err
	}
	cp, err := copilot.DefaultOptions()
	if err != nil {
		return nil, err
	}
	defaults := map[string]any{"editors": local.Defaults().Editors, "workspace": local.Defaults().Workspace, "repository_paths": []string{}, "jira": jira.Config{}, "copilot": cp, "refresh": defaultRefreshOptions(), "notifications": defaultNotificationOptions(), "github": map[string]any{"hidden_prefixes": defaultGitHubPrefixes}}
	b, _ := json.Marshal(defaults)
	var result map[string]any
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, err
	}
	// Merge recursively so partially configured sections retain defaults.
	var merge func(map[string]any, map[string]any)
	merge = func(dst, src map[string]any) {
		for k, v := range src {
			if nested, ok := v.(map[string]any); ok {
				if current, ok := dst[k].(map[string]any); ok {
					merge(current, nested)
					continue
				}
			}
			dst[k] = v
		}
	}
	raw, _ := json.Marshal(doc)
	var existing map[string]any
	if err := json.Unmarshal(raw, &existing); err != nil {
		return nil, err
	}
	merge(result, existing)
	return result, nil
}

func settingValue(doc map[string]any, path string) any {
	var value any = doc
	for _, part := range strings.Split(path, ".") {
		nested, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		value = nested[part]
	}
	return value
}
func (s *settingsState) loadValues() error {
	doc, err := settingsDocument()
	if err != nil {
		return err
	}
	s.values = nil
	for _, field := range settingFields {
		value := settingValue(doc, field.path)
		if value == nil {
			if field.kind == "list" || field.kind == "paths" || field.kind == "queries" {
				s.values = append(s.values, "[]")
			} else {
				s.values = append(s.values, "")
			}
		} else if text, ok := value.(string); ok {
			s.values = append(s.values, text)
		} else {
			raw, _ := json.Marshal(value)
			s.values = append(s.values, string(raw))
		}
	}
	return nil
}
func (m Model) openSettings() (tea.Model, tea.Cmd) {
	m.settings = settingsState{active: true}
	m.settings.input = textinput.New()
	m.settings.input.CharLimit = 32768
	m.settings.input.SetWidth(max(1, m.width-6))
	m.settings.input.Prompt = "> "
	m.queueSettings(nil, "")
	return m, nil
}
func parseSetting(field settingField, text string) (any, error) {
	value := strings.TrimSpace(text)
	switch field.kind {
	case "regex":
		if value != "" {
			if _, err := regexp.Compile(value); err != nil {
				return nil, err
			}
		}
	case "url":
		if value != "" {
			u, err := url.Parse(value)
			if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
				return nil, fmt.Errorf("bruk en HTTPS-adresse uten brukernavn, query eller fragment")
			}
		}
	case "path":
		if !filepath.IsAbs(value) {
			return nil, fmt.Errorf("bruk en absolutt filsti")
		}
	case "duration":
		d, err := time.ParseDuration(value)
		if err != nil || d < 5*time.Second || d > 24*time.Hour {
			return nil, fmt.Errorf("bruk et intervall fra 5s til 24h, f.eks. 1m")
		}
		return value, nil
	case "port":
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("port må være 1–65535")
		}
		return n, nil
	case "bool":
		if value != "true" && value != "false" {
			return nil, fmt.Errorf("skriv true eller false")
		}
		return value == "true", nil
	case "editor":
		if value != "intellij" && value != "vscode" && value != "custom" {
			return nil, fmt.Errorf("velg intellij, vscode eller custom")
		}
	case "list", "paths":
		var values []string
		if err := json.Unmarshal([]byte(value), &values); err != nil || values == nil {
			return nil, fmt.Errorf("bruk en JSON-liste, f.eks. [\"/sti\"]")
		}
		if field.kind == "paths" {
			for _, path := range values {
				if !filepath.IsAbs(path) {
					return nil, fmt.Errorf("repository paths må være absolutte")
				}
			}
		}
		return values, nil
	case "queries":
		var queries []jira.Query
		if err := json.Unmarshal([]byte(value), &queries); err != nil || queries == nil {
			return nil, fmt.Errorf("bruk en JSON-liste med name og jql")
		}
		for _, q := range queries {
			if q.Name == "" || q.JQL == "" {
				return nil, fmt.Errorf("hvert søk trenger name og jql")
			}
		}
		return queries, nil
	}
	return value, nil
}
func saveSetting(field settingField, text string) error {
	value, err := parseSetting(field, text)
	if err != nil {
		return err
	}
	doc, err := appconfig.Read(appconfig.Path())
	if err != nil {
		return err
	}
	// Change only this field, preserving all other sections and unknown fields.
	var current map[string]any
	raw, _ := json.Marshal(doc)
	if err := json.Unmarshal(raw, &current); err != nil {
		return err
	}
	parts := strings.Split(field.path, ".")
	nested := current
	for _, part := range parts[:len(parts)-1] {
		next, ok := nested[part].(map[string]any)
		if !ok {
			if existing, exists := nested[part]; exists && existing != nil {
				return fmt.Errorf("%s må være et JSON-objekt", part)
			}
			next = map[string]any{}
			nested[part] = next
		}
		nested = next
	}
	nested[parts[len(parts)-1]] = value
	return appconfig.Write(appconfig.Path(), current)
}
func (m Model) updateSettings(msg tea.Msg) (tea.Model, tea.Cmd) {
	s := &m.settings
	if key, ok := msg.(tea.KeyPressMsg); ok {
		if s.help && key.String() != "ctrl+c" && key.String() != "esc" {
			switch key.String() {
			case "?":
				s.help = false
			case "j", "down":
				lines := strings.Split(ansi.Wrap(strings.Join(settingsTokenHelp, "\n"), max(1, m.width), ""), "\n")
				s.helpOffset = min(s.helpOffset+1, max(0, len(lines)-max(1, m.height-4)))
			case "k", "up":
				s.helpOffset = max(0, s.helpOffset-1)
			}
			return m, nil
		}
		if !s.editing && key.String() == "?" {
			s.help, s.helpOffset = true, 0
			return m, nil
		}
		switch key.String() {
		case "ctrl+c":
			return m, m.quit()
		case "esc":
			if s.help {
				s.help = false
				return m, nil
			}
			if s.editorPicker {
				s.editorPicker = false
				return m, nil
			}
			if s.editing {
				s.editing = false
				s.chooseCustom = false
				s.input.Blur()
			} else {
				s.active = false
			}
			return m, nil
		case "enter":
			if s.busy {
				return m, nil
			}
			if s.editorPicker {
				value := []string{"intellij", "vscode", "custom"}[s.editorChoice]
				if value == "custom" {
					s.editorPicker = false
					s.chooseCustom = true
					s.selected = 10
					s.input.SetValue(s.values[s.selected])
					s.editing = true
					s.notice = "Skriv editorens launcher. Argumenter kan endres i neste felt."
					return m, s.input.Focus()
				}
				field := settingFields[s.selected]
				m.queueSettings(&field, value)
				return m, nil
			}
			if !s.editing {
				if len(s.values) != len(settingFields) {
					m.queueSettings(nil, "")
					return m, nil
				}
				if settingFields[s.selected].kind == "editor" {
					s.editorPicker = true
					s.editorChoice = 0
					for i, value := range []string{"intellij", "vscode", "custom"} {
						if value == s.values[s.selected] {
							s.editorChoice = i
						}
					}
					return m, nil
				}
				s.input.SetValue(s.values[s.selected])
				s.editing = true
				s.notice = ""
				return m, s.input.Focus()
			}
			field := settingFields[s.selected]
			if field.path == "editors.custom.command" && strings.TrimSpace(s.input.Value()) == "" {
				s.notice = "Skriv en kommando for editoren."
				return m, nil
			}
			m.queueSettings(&field, s.input.Value())
			return m, nil
		}
		if s.editorPicker {
			switch key.String() {
			case "j", "down":
				s.editorChoice = (s.editorChoice + 1) % 3
			case "k", "up":
				s.editorChoice = (s.editorChoice + 2) % 3
			}
			return m, nil
		}
		if !s.editing {
			switch key.String() {
			case "j", "down":
				s.selected = (s.selected + 1) % len(settingFields)
			case "k", "up":
				s.selected = (s.selected + len(settingFields) - 1) % len(settingFields)
			}
			return m, nil
		}
	}
	if s.editing {
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

var settingsTokenHelp = []string{
	"Tokenene leses ved oppstart. Start appen på nytt etter oppsett.",
	"GitHub: Logg inn med GitHub CLI:",
	"  gh auth login --hostname github.com",
	"  gh auth refresh --hostname github.com -s repo -s read:org",
	"Autoriser SAML SSO hvis organisasjonen krever det.",
	"Alternativ: sett GITHUB_TOKEN til et PAT med tilsvarende tilgang.",
	"GITHUB_TOKEN overstyrer CLI-innlogging. Fjern med: unset GITHUB_TOKEN",
	"",
	"Jira: Opprett et personlig tilgangstoken (PAT) i Jira-profilen.",
	"Administrator må ha tillatt PAT. Appen bruker Bearer-token / REST API v2.",
	"Sett Jira · serveradresse til f.eks. https://jira.example.no/jira",
	"Sett JIRA_TOKEN til PAT-et via din hemmelighetshåndtering.",
	"Bash: Les inn token uten å vise det eller lagre det i shell-historikken:",
	`  read -rsp 'Jira PAT: ' JIRA_TOKEN; echo; export JIRA_TOKEN`,
	"Kjør custom-visning --jira fra samme terminal.",
	"Samme Bash-kommando kan brukes med GITHUB_TOKEN i stedet for JIRA_TOKEN.",
	"",
	"Mattermost-lesing: Sett serveradresse og brukernavn i innstillingene.",
	"Åpne chat med vc; al logger inn med passord/TOTP, ap bruker PAT.",
	"Første oppsett kan tas i bruk med vc uten omstart; kontobytte krever omstart.",
	"",
	"Mattermost-sending: Opprett en incoming-webhook for ønsket kanal.",
	"Sett MATTERMOST_WEBHOOK_URL til hele HTTPS-adressen via samme metode.",
	"Webhook-adressen inneholder en hemmelighet og behandles som et token.",
	"",
	"Miljøvariabler gjelder prosessen som starter appen; bruk samme terminal.",
	"Ikke legg token i config.json eller kildekoden. Hjelpen viser aldri tokenverdier.",
}

func (m Model) settingsView() tea.View {
	s := m.settings
	lines := []string{accent.Render(" INNSTILLINGER"), "Felles konfigurasjon: " + clean(appconfig.Path()), ""}
	if s.help {
		lines = []string{accent.Render(" TOKENHJELP"), ""}
		wrapped := strings.Split(ansi.Wrap(strings.Join(settingsTokenHelp, "\n"), max(1, m.width), ""), "\n")
		capacity := max(1, m.height-4)
		start := min(s.helpOffset, max(0, len(wrapped)-capacity))
		lines = append(lines, wrapped[start:min(len(wrapped), start+capacity)]...)
		lines = append(lines, "", "↑/↓ eller j/k rull · ?/Esc tilbake")
		view := tea.NewView(strings.Join(lines, "\n"))
		view.AltScreen = true
		view.WindowTitle = "custom-visning · Tokenhjelp"
		return view
	}
	capacity := max(1, m.height-12)
	start := max(0, s.selected-capacity+1)
	for i := start; i < min(len(settingFields), start+capacity); i++ {
		value := ""
		if i < len(s.values) {
			value = s.values[i]
		}
		row := "  " + settingFields[i].label + ": " + clean(value)
		if i == s.selected {
			row = selection.Render("> " + strings.TrimSpace(row))
		}
		lines = append(lines, row)
	}
	if s.editorPicker {
		lines = append(lines, "", "Velg standardeditor:")
		for i, label := range []string{"IntelliJ IDEA", "VS Code", "Annen · egen kommando"} {
			prefix := "  "
			if i == s.editorChoice {
				prefix = "> "
			}
			lines = append(lines, prefix+label)
		}
	}
	if s.editing {
		lines = append(lines, "", settingFields[s.selected].label, s.input.View())
	}
	if settingFields[s.selected].path == "jira.base_url" {
		lines = append(lines, "Eksempel: https://jira.example.no/jira (inkluder eventuell /jira-sti)")
	}
	if s.selected == 0 {
		lines = append(lines, "Eksempel: KAKE-[0-9]{3,5} · tom verdi bruker standardmønsteret")
	}
	lines = append(lines, warning.Render(clean(s.notice)), "Miljøvariabler overstyrer filen ved oppstart. Token settes i miljøet.", "? tokenhjelp · ↑/↓ eller j/k velg · Enter rediger/lagre · Esc tilbake")
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, max(1, m.width), "…")
	}
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	view.WindowTitle = "custom-visning · Innstillinger"
	return view
}

// A request identity prevents an old load from changing a reopened editor.
type settingsRequest struct{ marker bool }

func (m *Model) queueSettings(field *settingField, value string) {
	request := &settingsRequest{}
	m.settings.request, m.settings.busy = request, true
	m.settings.notice = "Laster…"
	if field != nil {
		m.settings.notice = "Lagrer…"
	}
	var loaded settingsState
	var runtime Model
	var config jira.Config
	var editors local.Config
	chooseCustom := m.settings.chooseCustom
	m.enqueueLocal(func() error {
		if field != nil {
			if err := saveSetting(*field, value); err != nil {
				return err
			}
		}
		if field != nil && field.path == "editors.custom.command" && chooseCustom {
			if err := saveSetting(settingFields[5], "custom"); err != nil {
				return err
			}
		}
		if field != nil && strings.HasPrefix(field.path, "editors.") {
			var editorErr error
			editors, editorErr = local.LoadConfig(appconfig.Path())
			if editorErr != nil {
				return editorErr
			}
		}
		if err := loaded.loadValues(); err != nil {
			return err
		}
		runtime.loadRefresh()
		runtime.loadNotifications()
		runtime.loadGitHubDisplay()
		runtime.loadJiraPattern()
		var err error
		config, err = jira.LoadConfig(jira.ConfigPath())
		return err
	}, func(m *Model, err error) {
		// Saved configuration still applies if the user closed the editor meanwhile.
		if err == nil && field != nil {
			if strings.HasPrefix(field.path, "editors.") && m.local != nil {
				m.local.launcher.Editors = editors.Editors
			}
			switch {
			case strings.HasPrefix(field.path, "refresh."):
				m.auto.options = runtime.auto.options
			case strings.HasPrefix(field.path, "notifications."):
				m.notifications.options = runtime.notifications.options
			case field.path == "github.hidden_prefixes":
				m.githubHiddenPrefixes = runtime.githubHiddenPrefixes
			case field.path == "jira.pattern":
				m.jiraPattern = runtime.jiraPattern
				m.selected = 0
			}
			if m.jira != nil {
				m.jira.config = config
				m.jira.tabs = append(m.jira.tabs[:4:4], config.Queries...)
				m.jira.active = min(m.jira.active, len(m.jira.tabs)-1)
			}
		}
		if m.settings.request != request {
			return
		}
		m.settings.busy = false
		if err != nil {
			m.settings.notice = "Kunne ikke lese/lagre innstillinger: " + err.Error()
			return
		}
		m.settings.values = loaded.values
		m.settings.notice = ""
		if field != nil {
			m.settings.editing = false
			m.settings.editorPicker = false
			m.settings.chooseCustom = false
			m.settings.input.Blur()
			m.settings.notice = "Lagret. Øvrige innstillinger tas i bruk etter omstart."
			switch {
			case strings.HasPrefix(field.path, "editors."):
				m.settings.notice = "Editor lagret og tatt i bruk."
			case strings.HasPrefix(field.path, "mattermost.") && m.mmReader == nil:
				m.settings.notice = "Lagret. Esc tilbake og vc tar førstegangsoppsettet i bruk uten omstart."
			case strings.HasPrefix(field.path, "notifications."):
				m.settings.notice = "Varslingsinnstillinger tatt i bruk."
			case strings.HasPrefix(field.path, "refresh."):
				m.settings.notice = "Oppdateringsinnstillinger tatt i bruk."
			case field.path == "github.hidden_prefixes":
				m.settings.notice = "GitHub-visning lagret og tatt i bruk."
			case field.path == "jira.pattern":
				m.settings.notice = "Jira-mønster lagret og tatt i bruk."
			}
		}
	})
}
