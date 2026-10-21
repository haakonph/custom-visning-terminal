package tui

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	appconfig "github.com/haakonph/custom-visning-terminal/internal/config"
	"github.com/haakonph/custom-visning-terminal/internal/inputsource"
)

type keyboardOptions struct {
	Profile        string            `json:"profile"`
	QwertyHome     string            `json:"qwerty_home"`
	WorkmanHome    string            `json:"workman_home"`
	SourceProfiles map[string]string `json:"source_profiles"`
}

func defaultKeyboardOptions() keyboardOptions {
	return keyboardOptions{
		Profile: "auto", QwertyHome: "asdf jklø", WorkmanHome: "ciae rhtns",
		SourceProfiles: map[string]string{},
	}
}

type keyboardState struct {
	options        keyboardOptions
	profile        string
	encode, decode map[rune]rune
	source         inputsource.Source
	sourceError    string
	currentSource  func() (inputsource.Source, error)
}

// Frequent commands occupy the finger and thumb resting positions. Complete
// the partial mapping into a permutation, so every other shortcut stays usable
// and no key can accidentally invoke two commands.
func homeKeyMap(home string) (map[rune]rune, error) {
	keys := []rune(home)
	if (len(keys) != 9 && len(keys) != 10) || keys[4] != ' ' {
		return nil, fmt.Errorf("bruk fire venstretaster, mellomrom, og fire eller fem høyretaster, f.eks. asdf jklø eller ciae rhtns")
	}
	anchors := []rune("avde jkhl")
	if len(keys) == 10 {
		anchors = []rune("avde rjkhl")
	}
	encode, inverse := map[rune]rune{}, map[rune]rune{}
	for i, key := range keys {
		if (!unicode.IsLetter(key) && key != ';' && !(i == 4 && key == ' ')) || unicode.IsUpper(key) {
			return nil, fmt.Errorf("bruk små bokstaver eller semikolon som hviletaster")
		}
		if _, exists := inverse[key]; exists {
			return nil, fmt.Errorf("hver hviletast må være unik")
		}
		encode[anchors[i]], inverse[key] = key, anchors[i]
	}
	for _, key := range keys {
		if _, assigned := encode[key]; assigned {
			continue
		}
		start := key
		for {
			previous, exists := inverse[start]
			if !exists {
				break
			}
			start = previous
		}
		encode[key] = start
	}
	return encode, nil
}

func (m *Model) loadKeyboard() {
	options := defaultKeyboardOptions()
	if err := appconfig.Section(appconfig.Path(), "keyboard", &options); err != nil {
		m.notice = "Kunne ikke lese tastaturinnstillinger: " + err.Error()
		return
	}
	for _, home := range []string{options.QwertyHome, options.WorkmanHome} {
		if _, err := homeKeyMap(home); err != nil {
			m.notice = "Ugyldig tastaturinnstilling: " + err.Error()
			return
		}
	}
	if options.Profile != "auto" && options.Profile != "standard" && options.Profile != "qwerty" && options.Profile != "workman" {
		m.notice = "Tastaturprofil må være auto, standard, qwerty eller workman"
		return
	}
	for id, profile := range options.SourceProfiles {
		if id == "" || (profile != "standard" && profile != "qwerty" && profile != "workman") {
			m.notice = "Ugyldig tastaturprofil i keyboard.source_profiles"
			return
		}
	}
	m.keyboard.options = options
	profile := options.Profile
	if profile == "auto" {
		profile = options.SourceProfiles[m.keyboard.source.ID]
		if profile == "" {
			profile = "standard"
		}
	}
	m.setKeyboardProfile(profile)
}

func (m *Model) setKeyboardProfile(profile string) {
	if profile != m.keyboard.profile {
		m.sequence, m.sequenceNotice, m.sequenceRefs = "", "", nil
		if m.jiraAction != nil && !m.jiraAction.running {
			m.jiraAction = nil
		}
	}
	m.keyboard.profile = profile
	m.keyboard.encode, m.keyboard.decode = nil, nil
	var home string
	switch profile {
	case "qwerty":
		home = m.keyboard.options.QwertyHome
	case "workman":
		home = m.keyboard.options.WorkmanHome
	default:
		return
	}
	encode, err := homeKeyMap(home)
	if err != nil {
		m.notice = err.Error()
		return
	}
	m.keyboard.encode, m.keyboard.decode = encode, map[rune]rune{}
	for from, to := range encode {
		m.keyboard.decode[to] = from
	}
}

func translateKey(key rune, mapping map[rune]rune) rune {
	lower := unicode.ToLower(key)
	if mapped, exists := mapping[lower]; exists {
		if unicode.IsUpper(key) {
			return unicode.ToUpper(mapped)
		}
		return mapped
	}
	return key
}

func (m Model) shortcut(keys string) string {
	return strings.Map(func(key rune) rune { return translateKey(key, m.keyboard.encode) }, keys)
}

// Backticks mark shortcut sequences in application-authored prose. Never pass
// user/provider text through this formatter.
func (m Model) keyText(text string) string {
	parts := strings.Split(text, "`")
	for i := 1; i < len(parts); i += 2 {
		parts[i] = m.shortcut(parts[i])
	}
	return strings.Join(parts, "")
}

// Apply only to shortcut hints written as "keys description · keys description".
// Descriptions, user content, URLs, tokens and text inputs are never translated.
func (m Model) keyHints(hints string) string {
	if len(m.keyboard.encode) == 0 {
		return hints
	}
	parts := strings.Split(hints, " · ")
	for i, part := range parts {
		leading := len(part) - len(strings.TrimLeft(part, " "))
		end := strings.IndexByte(part[leading:], ' ')
		if end < 0 {
			continue
		}
		end += leading
		tokens := strings.Split(part[leading:end], "/")
		for j, token := range tokens {
			if token == "" || token == "Space" || token == "Esc" || token == "Enter" || strings.HasPrefix(token, ":") || strings.Contains(token, "Ctrl+") || strings.Contains(token, "Shift+") || (len(token) > 1 && unicode.IsUpper([]rune(token)[0])) {
				continue
			}
			tokens[j] = m.shortcut(token)
		}
		parts[i] = part[:leading] + strings.Join(tokens, "/") + part[end:]
	}
	return strings.Join(parts, " · ")
}

func (m Model) keyboardCommandMode() bool {
	if m.internalKey || m.setup.wizard != nil || (m.snoozes != nil && m.snoozes.input) || (m.local != nil && len(m.local.removePending) > 0) {
		return false
	}
	if m.settings.active {
		return !m.settings.editing
	}
	if m.local != nil && m.local.choosing {
		return true
	}
	if m.sequence != "" || m.jiraAction != nil || m.help || m.setup.provider != "" {
		return true
	}
	if m.mmReader != nil && m.mmReader.active && m.mmReader.choosing {
		return !m.mmReader.channelSearching
	}
	return m.keyboardNavigation()
}

func (m Model) profileKey(key tea.KeyPressMsg) tea.KeyPressMsg {
	if !m.keyboardCommandMode() || key.Mod&^(tea.ModShift|tea.ModCapsLock) != 0 || utf8.RuneCountInString(key.String()) != 1 {
		return key
	}
	r, _ := utf8.DecodeRuneInString(key.String())
	mapped := translateKey(r, m.keyboard.decode)
	if r != mapped {
		key.Code, key.Text, key.ShiftedCode, key.BaseCode = mapped, string(mapped), 0, 0
		key.Mod = 0
	}
	return key
}

type keyboardSourceMsg struct {
	source inputsource.Source
	err    error
}

func readInputSource() tea.Cmd {
	if runtime.GOOS != "darwin" {
		return nil
	}
	return func() tea.Msg {
		source, err := inputsource.Current()
		return keyboardSourceMsg{source, err}
	}
}

func (m *Model) applyInputSource(source inputsource.Source, err error) {
	m.keyboard.sourceError = ""
	if err != nil {
		m.keyboard.sourceError = err.Error()
		m.keyboard.source = inputsource.Source{}
		return
	}
	m.keyboard.source = source
	if m.keyboard.options.Profile == "auto" {
		profile := m.keyboard.options.SourceProfiles[source.ID]
		if profile == "" {
			profile = "standard"
		}
		if profile != m.keyboard.profile {
			m.setKeyboardProfile(profile)
		}
	}
}

func (m Model) keyboardMessage(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	if source, ok := msg.(keyboardSourceMsg); ok {
		m.applyInputSource(source.source, source.err)
		return m, tea.Tick(time.Second, func(time.Time) tea.Msg {
			source, err := inputsource.Current()
			return keyboardSourceMsg{source, err}
		}), true
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok || m.internalKey || key.String() != "f2" {
		return m, nil, false
	}
	profile := "qwerty"
	if m.keyboard.profile == "qwerty" {
		profile = "workman"
	}
	// In automatic mode F2 teaches the current source once. Later source
	// changes select its saved profile, including firmware-programmed layouts.
	field, value := settingField{path: "keyboard.profile", kind: "keyboard-profile"}, profile
	if m.keyboard.options.Profile == "auto" && m.keyboard.source.ID != "" {
		profiles := map[string]string{}
		for id, existing := range m.keyboard.options.SourceProfiles {
			profiles[id] = existing
		}
		profiles[m.keyboard.source.ID] = profile
		if err := saveSourceProfiles(profiles); err != nil {
			m.notice = "Kunne ikke lagre tastaturprofil: " + err.Error()
			return m, nil, true
		}
	} else if err := saveSetting(field, value); err != nil {
		m.notice = "Kunne ikke lagre tastaturprofil: " + err.Error()
		return m, nil, true
	}
	m.loadKeyboard()
	m.notice = "Tastaturprofil: " + m.keyboardLabel()
	if m.settings.active {
		m.settings.notice = m.notice
		if !m.settings.editing {
			if err := m.settings.loadValues(); err != nil {
				m.settings.notice = err.Error()
			}
		}
	}
	return m, nil, true
}

func saveSourceProfiles(profiles map[string]string) error {
	// Use the shared field writer to preserve unknown keyboard settings too.
	raw, err := json.Marshal(profiles)
	if err != nil {
		return err
	}
	return saveSetting(settingField{path: "keyboard.source_profiles", kind: "keyboard-sources"}, string(raw))
}

func (m Model) keyboardLabel() string {
	profile := m.keyboard.profile
	if profile == "" {
		profile = "standard"
	}
	if m.keyboard.source.Name != "" {
		profile += " (" + clean(m.keyboard.source.Name) + ")"
	}
	return profile
}

func (m Model) shortcutFooter() string {
	return "F2 " + m.keyboardLabel() + " · " + m.keyHints(shortcuts)
}
