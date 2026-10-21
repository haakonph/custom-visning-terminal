package tui

import "strings"

type keyboardCommand struct {
	prefix, key, label string
}

// One list controls which commands can run and which commands help and menus show.
// Commands always use the current view; a target type cannot extend a command.
func (m Model) keyboardCommands() []keyboardCommand {
	var commands []keyboardCommand
	add := func(prefix, key, label string) {
		commands = append(commands, keyboardCommand{prefix, key, label})
	}
	if m.mattermost != nil && m.mattermost.active {
		add("e", "e", "rediger melding")
		add("a", "s", "send melding")
		return commands
	}
	for _, target := range []struct{ key, label string }{
		{"o", "oppgaver"}, {"g", "GitHub"}, {"t", "Jira"}, {"a", "AI"}, {"c", "chat"},
		{"s", "innstillinger"}, {"r", "forrige visning"}, {"z", "utsatte"}, {"b", "arkiv"}, {"h", "oppsett"},
	} {
		add("v", target.key, target.label)
	}
	if m.setup.provider != "" {
		return commands
	}
	kind := m.visibleProvider()
	if kind == "dialog" {
		if m.dialog.mode == "archive" || m.dialog.restore {
			add("a", "k", "gjenopprett fra arkiv")
		} else if m.dialog.mode == "deferred" || (m.dialog.mode == "snapshot" && m.snoozes.active) {
			add("a", "z", "hent tilbake")
		}
		return commands
	}
	add("a", "u", "oppdater visningen")
	if kind == "deferred" {
		add("a", "z", "hent tilbake")
		return commands
	}
	if kind == "copilot" && m.copilot.diagnosticsOpen {
		return commands
	}
	// Relations and local links are explicit navigation/selection dialogs. Actions
	// on a related item require opening that item's view first.
	for _, target := range []struct{ key, label string }{
		{"o", "oppgaver"}, {"g", "GitHub"}, {"t", "Jira"}, {"a", "AI"}, {"c", "chat"},
	} {
		add("r", target.key, "relaterte "+target.label)
		add("t", target.key, "koble til "+target.label)
	}
	add("a", "s", "lag meldingsutkast fra valget")
	add("a", "k", "arkiver valget lokalt")
	switch kind {
	case "github":
		add("a", "+", "automerge på")
		add("a", "-", "automerge av")
		add("a", "r", "be om Copilot-review")
		add("a", "w", "slett lokale worktrees")
		add("e", "e", "åpne i IntelliJ")
		if !m.detailsOpen() {
			add("e", "v", "velg editor")
		}
		add("e", "n", "åpne i nettleser")
	case "jira":
		add("a", "f", "flytt valgte saker")
		add("a", "m", "tilordne meg")
		add("a", "r", "fjern min tilordning")
		if !m.detailsOpen() {
			add("a", "q", "lagre søk")
			add("a", "x", "nullstill filtre og JQL")
			add("v", "f", "hurtigfiltre")
			add("v", "j", "egen JQL")
		}
		add("e", "n", "åpne i nettleser")
	case "copilot":
		add("v", "i", "diagnostikk")
		add("e", "e", "åpne chatmappe i IntelliJ")
		add("e", "w", "åpne worktree i IntelliJ")
	case "task":
		add("a", "n", "ny oppgave")
		if m.tasks.details {
			add("e", "n", "åpne valgt referanse i nettleser")
			add("a", "d", "slett valgte oppgavereferanser")
		} else {
			add("a", "e", "endre oppgavetittel")
			add("a", "d", "slett valgte oppgaver")
		}
	case "mattermost":
		add("v", "k", "velg kanaler")
		add("a", "l", "logg inn med passord")
		add("a", "p", "logg inn med PAT")
		add("a", "v", "merk kanal lest")
		add("a", "w", "slutt å overvåke kanal")
		add("a", "b", "tøm lokal chatkonto")
		add("a", "h", "hent eldre meldinger")
		add("a", "down", "rull en halv side ned")
		add("a", "up", "rull en halv side opp")
		add("e", "n", "åpne i nettleser")
		add("e", "l", "vis lenker i valget")
	}
	return commands
}

func (m Model) commandAllowed(prefix, key string) bool {
	for _, command := range m.keyboardCommands() {
		if command.prefix == prefix && command.key == key {
			return true
		}
	}
	return false
}

func (m Model) sequenceHint(prefix string) string {
	var hints []string
	for _, command := range m.keyboardCommands() {
		if command.prefix == prefix {
			hints = append(hints, command.key+" "+command.label)
		}
	}
	return strings.Join(hints, " · ")
}

func (m Model) detailsOpen() bool {
	switch m.visibleProvider() {
	case "github":
		return m.detail != nil
	case "jira":
		return m.jira.detail != nil
	case "copilot":
		return m.copilot.detail != nil || m.copilot.diagnosticsOpen
	case "dialog":
		return m.dialog.mode == "snapshot"
	}
	return false
}

func (m Model) canShowDetails() bool {
	if m.detailsOpen() || (m.visibleProvider() == "task" && m.tasks.details) {
		return false
	}
	return m.dialog == nil || m.dialog.mode == "archive" || m.dialog.mode == "deferred"
}

func (m Model) shortcuts() string {
	if !m.keyboardNavigation() {
		return "Enter bekreft · Esc avbryt · Ctrl+C avslutt"
	}
	parts := []string{"j/k ↑/↓ velg/rull"}
	if !m.detailsOpen() {
		if m.canShowDetails() {
			parts = append(parts, "d detaljer")
		}
		parts = append(parts, "Space marker", "/ søk")
	}
	for _, menu := range []struct{ prefix, label string }{{"v", "visning"}, {"a", "handling"}, {"e", "åpne"}, {"r", "relatert"}, {"t", "koble"}} {
		if m.sequenceHint(menu.prefix) != "" {
			parts = append(parts, menu.prefix+" "+menu.label)
		}
	}
	return strings.Join(append(parts, "? hjelp", "Esc tilbake", "Ctrl+C avslutt"), " · ")
}

func (m Model) helpText() string {
	kind := m.visibleProvider()
	lines := []string{"Hurtigtaster · " + kind, "j/k / ↑/↓   Velg rad eller rull i detaljer"}
	composing := m.mattermost != nil && m.mattermost.active
	switch {
	case composing:
		lines[0] = "Hurtigtaster · meldingsutkast"
	case m.setup.provider != "":
		lines = append(lines, "Enter       Prøv visningen igjen", "f           Konfigurer integrasjon", "PageUp/Down Rull veiledningen")
	case !m.detailsOpen():
		if kind == "github" || kind == "jira" || kind == "task" || kind == "mattermost" {
			lines = append(lines, "h/l / ←/→   Forrige/neste fane eller oppgave")
		}
		if m.canShowDetails() {
			lines = append(lines, "d           Detaljer; chat: tråd")
		}
		lines = append(lines, "/           Søk/filtrer i visningen", "Space       Marker/fjern markering", "Shift+↑/↓   Utvid markeringen", "x           Fjern markeringer")
		if m.dialog != nil {
			lines = append(lines, "Enter       Bekreft valg / åpne valgt element")
		}
	}
	if !composing && m.setup.provider == "" && m.dialog == nil && !(kind == "copilot" && m.copilot.diagnosticsOpen) {
		lines = append(lines, "z           Utsett valget; tall + h/d/m/D velger tid", "zz          Utsett hele sammenhengen; viser medlemmene først")
	}
	lines = append(lines, "? / Esc     Lukk hjelp / avbryt", "Ctrl+C      Avslutt", "", "Velg meny, deretter én handling:")
	for _, command := range m.keyboardCommands() {
		lines = append(lines, command.prefix+command.key+"  "+command.label)
	}
	return strings.Join(lines, "\n")
}
