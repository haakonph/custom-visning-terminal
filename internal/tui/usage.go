package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/haakonph/custom-visning-terminal/internal/usage"
)

func (m Model) WithUsage(recorder *usage.Recorder, notice string) Model {
	m.usage, m.usageNotice = recorder, notice
	return m
}

// Update observes only real key messages. Internal compatibility dispatch and
// background refreshes must never count as user activity.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, isKey := msg.(tea.KeyPressMsg)
	if m.usage == nil || !isKey || m.internalKey {
		return m.update(msg)
	}
	before := m.usageScreen()
	provider := m.baseProvider()
	cursor := m.cursor()
	tab := m.usageTab()
	task := m.tasks.selected
	category, name, target := m.usageCommand(key.String())
	next, cmd := m.update(msg)
	result := next.(Model)
	if category != "" && result.sequence == "" {
		m.usage.Record(category, name, provider, target)
	}
	if after := result.usageScreen(); before != after {
		m.usage.Record("navigation", "screen", before, after)
	} else if after := result.usageTab(); tab != after {
		m.usage.Record("navigation", "tab", before+":"+tab, before+":"+after)
	} else if provider == "task" && task != result.tasks.selected && (name == "tab.previous" || name == "tab.next") {
		m.usage.Record("navigation", "item", before, before)
	} else if (key.String() == "j" || key.String() == "k" || key.String() == "up" || key.String() == "down" || key.String() == "shift+up" || key.String() == "shift+down") && name != "" && cursor != result.cursor() {
		m.usage.Record("navigation", "row", before, before)
	}
	return result, cmd
}

func (m Model) usageScreen() string {
	switch {
	case m.local != nil && len(m.local.removePending) > 0:
		return "worktree.remove.confirm"
	case m.help:
		return "help"
	case m.usageView:
		return "metrics"
	case m.settings.active:
		return "settings"
	case m.setup.wizard != nil:
		return "setup.wizard"
	case m.setup.provider != "":
		return "setup"
	case m.snoozes != nil && m.snoozes.input:
		return "snooze"
	case m.mattermost != nil && m.mattermost.active:
		return "mattermost.compose"
	case m.dialog != nil:
		return "dialog." + m.dialog.mode
	case m.snoozes != nil && m.snoozes.active:
		return "deferred"
	case m.detail != nil:
		return "github.details"
	case m.local != nil && m.local.choosing:
		return "editor.choose"
	case m.tasks.active && m.tasks.details:
		return "task.details"
	case m.jiraActive && m.jira != nil && m.jira.detail != nil:
		return "jira.details"
	case m.copilotActive && m.copilot != nil && m.copilot.diagnosticsOpen:
		return "copilot.diagnostics"
	case m.copilotActive && m.copilot != nil && m.copilot.detail != nil:
		return "copilot.details"
	case m.mmReader != nil && m.mmReader.active:
		switch {
		case m.mmReader.mode != "":
			return "mattermost.login"
		case m.mmReader.choosing:
			return "mattermost.channels"
		case m.mmReader.confirmClear:
			return "mattermost.clear.confirm"
		case m.mmReader.root != "":
			return "mattermost.thread"
		}
	}
	return m.baseProvider() + ".list"
}

func (m Model) usageTab() string {
	switch m.baseProvider() {
	case "github":
		return m.active.String()
	case "jira":
		if m.jira != nil {
			return fmt.Sprint(m.jira.active)
		}
	case "mattermost":
		// Channel names and IDs deliberately stay out of metrics.
		if m.mmReader != nil {
			return fmt.Sprint(m.mmReader.tab)
		}
	}
	return ""
}

// Labels come from this allowlist; key text and entity metadata are never saved.
// Commands count requests, including unavailable or subsequently failed actions.
func (m Model) usageCommand(k string) (category, name, target string) {
	if m.usageView || m.help || m.settings.active || m.setup.provider != "" || m.setup.wizard != nil {
		return "", "", ""
	}
	if m.jiraAction != nil {
		if k == "enter" || k == "q" || k == "u" || k == "a" || k == "f" {
			return "command", "jira.transition.confirm", "jira"
		}
		return "", "", ""
	}
	if m.sequence != "" {
		if k == "esc" {
			return "command", "sequence.cancel", ""
		}
		op := m.sequence[:1]
		if op == "v" {
			target = map[string]string{"o": "task", "g": "github", "t": "jira", "a": "copilot", "c": "mattermost", "s": "settings", "z": "deferred", "b": "archive", "i": "diagnostics", "f": "jira.filters", "j": "jira.jql", "k": "mattermost.channels", "h": "setup", "m": "metrics", "r": "previous"}[k]
			if target != "" {
				return "command", "view", target
			}
		}
		if op == "r" || op == "t" {
			if target = provider(k); target != "" {
				name = "related"
				if op == "t" {
					name = "link.choose"
				}
				return "command", name, target
			}
		}
		if op == "e" {
			name = map[string]string{"e": "open.editor", "n": "open.browser", "v": "editor.choose", "l": "links.choose"}[k]
			if m.sequence == "ee" {
				name = map[string]string{"e": "open.editor.directory", "w": "open.editor.worktree"}[k]
			}
		}
		if op == "a" {
			name = map[string]string{"+": "automerge.enable", "-": "automerge.disable", "f": "jira.transition.choose", "m": "jira.assign", "r": "jira.unassign", "s": "message", "k": "archive", "d": "delete.local", "u": "refresh", "n": "task.new", "e": "task.rename", "x": "jira.filters.clear", "q": "jira.query.save", "l": "mattermost.login.password", "p": "mattermost.login.token", "h": "mattermost.older", "v": "mattermost.read", "w": "worktree.remove", "b": "mattermost.clear", "down": "scroll.page.down", "up": "scroll.page.up", "z": "snooze.restore"}[k]
			if k == "r" && allProvider(m.sequenceRefs, "github") {
				name = "review.request"
			}
			if k == "w" && m.visibleProvider() == "mattermost" {
				name = "mattermost.channel.remove"
			}
		}
		if name != "" {
			kinds := map[string]bool{}
			for _, ref := range m.sequenceRefs {
				if ref.Provider == "task" || ref.Provider == "github" || ref.Provider == "jira" || ref.Provider == "copilot" || ref.Provider == "mattermost" || ref.Provider == "external" {
					kinds[ref.Provider] = true
				}
			}
			var labels []string
			for kind := range kinds {
				labels = append(labels, kind)
			}
			sort.Strings(labels)
			return "command", name, strings.Join(labels, "+")
		}
		return "", "", ""
	}
	if !m.keyboardNavigation() || (m.local != nil && len(m.local.removePending) > 0) {
		return "", "", ""
	}
	name = map[string]string{"j": "move.down", "down": "move.down", "k": "move.up", "up": "move.up", "h": "tab.previous", "left": "tab.previous", "l": "tab.next", "right": "tab.next", "d": "details", "/": "search", "?": "help", "space": "select.toggle", " ": "select.toggle", "shift+up": "select.up", "shift+down": "select.down", "x": "select.clear", "esc": "back", "z": "snooze.begin"}[k]
	if k == "enter" && m.dialog != nil {
		name = "dialog.confirm"
	}
	if name != "" {
		return "command", name, ""
	}
	return "", "", ""
}

func (m Model) usageStatsLines() []string {
	periods := []int{7, 30, 0}
	labels := []string{"7 dager", "30 dager", "hele perioden"}
	lines := []string{accent.Render(" BRUKSSTATISTIKK · " + labels[m.usagePeriod]), "h/l periode · j/k rull · Esc tilbake", "Lokale tellere. Kommandoer teller forespørsler, også ved avbrudd eller feil.", ""}
	if m.usage == nil {
		lines = append(lines, "Innsamling er ikke tilgjengelig. "+m.usageNotice)
	} else {
		counts, err := m.usage.Snapshot(periods[m.usagePeriod])
		if err != nil {
			lines = append(lines, "Kunne ikke lagre statistikk; nye tellere finnes foreløpig bare i minnet.")
		}
		if len(counts) == 0 {
			lines = append(lines, "Ingen registrert bruk i perioden.")
		}
		category := ""
		for _, c := range counts {
			if c.Category != category {
				category = c.Category
				heading := map[string]string{"command": "Kommandoer", "navigation": "Faktisk navigasjon", "session": "Oppstarter"}[category]
				lines = append(lines, "", heading)
			}
			path := c.Source
			if c.Target != "" {
				path += " → " + c.Target
			}
			label := usageLabels[c.Name]
			if label == "" {
				label = c.Name
			}
			lines = append(lines, fmt.Sprintf("%7d  %-26s %s", c.Count, label, path))
		}
	}
	return lines
}

func (m Model) usageMaxOffset() int {
	return max(0, len(m.usageStatsLines())-max(4, m.height))
}

func (m Model) usageStatsView() tea.View {
	lines := m.usageStatsLines()
	header := lines[:min(4, len(lines))]
	body := lines[len(header):]
	available := max(0, m.height-len(header))
	start := min(m.usageOffset, max(0, len(body)-available))
	visible := append(append([]string{}, header...), body[start:min(len(body), start+available)]...)
	visible = visible[:min(len(visible), max(1, m.height))]
	for i := range visible {
		visible[i] = ansi.Truncate(visible[i], max(1, m.width), "…")
	}
	view := tea.NewView(strings.Join(visible, "\n"))
	view.AltScreen = true
	return view
}

var usageLabels = map[string]string{
	"start": "Appstart", "screen": "Visningsbytte", "tab": "Fanebytte", "row": "Radflytting", "item": "Oppgavebytte",
	"view": "Åpne visning", "related": "Vis relaterte", "link.choose": "Velg koblingsmål",
	"open.editor": "Åpne editor", "open.browser": "Åpne nettleser", "editor.choose": "Velg editor", "links.choose": "Vis lenker",
	"open.editor.directory": "Åpne arbeidsmappe", "open.editor.worktree": "Åpne worktree",
	"automerge.enable": "Automerge på", "automerge.disable": "Automerge av", "review.request": "Be om Copilot-review",
	"jira.transition.choose": "Velg Jira-overgang", "jira.transition.confirm": "Bekreft Jira-overgang", "jira.assign": "Tilordne Jira til meg", "jira.unassign": "Fjern Jira-tilordning",
	"message": "Melding", "archive": "Arkiver / gjenopprett", "delete.local": "Slett lokalt", "refresh": "Oppdater",
	"task.new": "Ny oppgave", "task.rename": "Endre oppgavetittel", "jira.filters.clear": "Nullstill Jira-filtre", "jira.query.save": "Lagre Jira-søk",
	"mattermost.login.password": "Logg inn med passord", "mattermost.login.token": "Logg inn med PAT", "mattermost.older": "Hent eldre meldinger", "mattermost.read": "Merk kanal lest",
	"worktree.remove": "Fjern worktree", "mattermost.channel.remove": "Slutt å overvåke kanal", "mattermost.clear": "Tøm lokal chatkonto",
	"scroll.page.down": "Rull side ned", "scroll.page.up": "Rull side opp", "snooze.restore": "Hent tilbake utsatte",
	"move.down": "Naviger ned", "move.up": "Naviger opp", "tab.previous": "Forrige fane", "tab.next": "Neste fane", "details": "Vis detaljer",
	"search": "Søk", "help": "Vis hjelp", "select.toggle": "Endre markering", "select.up": "Marker oppover", "select.down": "Marker nedover", "select.clear": "Fjern markeringer",
	"back": "Tilbake", "snooze.begin": "Utsett", "dialog.confirm": "Bekreft valg", "sequence.cancel": "Avbryt tastsekvens",
}
