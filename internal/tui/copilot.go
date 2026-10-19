package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/copilot"
)

type copilotState struct {
	entities *entityState
	snoozes  *snoozeState

	client                              *copilot.Service
	sessions                            []copilot.Session
	detail                              *copilot.Detail
	diagnostics                         copilot.Diagnostics
	input                               textinput.Model
	selected, offset                    int
	filtering, loading, diagnosticsOpen bool
	timerPending                        bool
	notice                              string
	openIDE                             func(context.Context, copilot.Session) error
	openingIDE                          bool
	ideNotice                           string
}
type copilotResult struct {
	sessions    []copilot.Session
	detail      *copilot.Detail
	diagnostics copilot.Diagnostics
	id          string
	err         error
}
type copilotTick struct{}
type copilotOpenMsg struct{ err error }

func (m Model) WithCopilot(client *copilot.Service, active bool) Model {
	input := textinput.New()
	input.Prompt = "/ "
	input.Placeholder = "session, repository, Jira, model, status"
	input.CharLimit = 256
	m.copilot = &copilotState{entities: m.entities, snoozes: m.snoozes, client: client, input: input, openIDE: openIntelliJ}
	m.copilotActive = active
	if active {
		m.jiraActive = false
		m.booting = false
	}
	return m
}
func (m Model) switchCopilot() (tea.Model, tea.Cmd) {
	if m.copilot == nil {
		m.notice = "Copilot is not configured"
		return m, nil
	}
	m.copilotActive = true
	m.jiraActive = false
	return m, m.refreshCopilot()
}
func (m *Model) refreshCopilot() tea.Cmd {
	c := m.copilot
	if c == nil || c.loading {
		return nil
	}
	c.loading = true
	client, ctx := c.client, m.ctx
	id := ""
	if c.detail != nil {
		id = c.detail.Session.ID
	}
	return func() tea.Msg {
		client.Scan(ctx)
		sessions, err := client.Sessions(ctx)
		result := copilotResult{sessions: sessions, id: id, err: err}
		if err == nil {
			result.diagnostics, result.err = client.Diagnostics(ctx)
		}
		if result.err == nil && id != "" {
			d, e := client.Detail(ctx, id)
			result.detail = &d
			result.err = e
		}
		return result
	}
}
func (c *copilotState) rows() []copilot.Session {
	q := strings.ToLower(strings.TrimSpace(c.input.Value()))
	rows := []copilot.Session{}
	for _, s := range c.sessions {
		ref := copilotRef(s)
		if c.entities.archived(ref.Provider, ref.URL) || (c.snoozes != nil && c.snoozes.hidden(refSnoozeID(ref))) {
			continue
		}
		if strings.Contains(strings.ToLower(strings.Join([]string{s.ID, s.Name, s.Client, s.Repository, s.ProjectRoot, s.Branch, s.Jira, s.Model, s.Agent, s.Status}, " ")), q) {
			rows = append(rows, s)
		}
	}
	return rows
}
func (m Model) updateCopilot(msg tea.Msg) (tea.Model, tea.Cmd) {
	c := m.copilot
	if c == nil {
		return m, nil
	}
	switch msg := msg.(type) {
	case copilotOpenMsg:
		c.openingIDE = false
		if msg.err != nil {
			c.ideNotice = msg.err.Error()
		} else {
			c.ideNotice = "Åpneforespørsel sendt til IntelliJ for sesjonens prosjekt."
		}
	case copilotTick:
		c.timerPending = false
		if m.copilotActive {
			return m, m.refreshCopilot()
		}
		return m, nil
	case copilotResult:
		c.loading = false
		if msg.err != nil {
			c.notice = msg.err.Error() + " · Previous data retained"
		} else {
			id := ""
			if rows := c.rows(); c.selected < len(rows) {
				id = rows[c.selected].ID
			}
			c.sessions = msg.sessions
			c.diagnostics = msg.diagnostics
			c.notice = ""
			for i, s := range c.rows() {
				if s.ID == id {
					c.selected = i
					break
				}
			}
			c.selected = max(0, min(c.selected, len(c.rows())-1))
			if msg.id != "" && c.detail != nil && c.detail.Session.ID == msg.id {
				c.detail = msg.detail
			}
		}
		if m.copilotActive && !c.timerPending {
			c.timerPending = true
			return m, tea.Tick(3*time.Second, func(time.Time) tea.Msg { return copilotTick{} })
		}
	case tea.KeyPressMsg:
		key := msg.String()
		if key == "ctrl+c" {
			m.cancel()
			return m, tea.Quit
		}
		if c.filtering {
			switch key {
			case "esc":
				c.input.SetValue("")
				c.filtering = false
				c.input.Blur()
			case "enter":
				c.filtering = false
				c.input.Blur()
			default:
				var cmd tea.Cmd
				c.input, cmd = c.input.Update(msg)
				c.selected = 0
				return m, cmd
			}
			return m, nil
		}
		switch key {
		case "q":
			m.cancel()
			return m, tea.Quit
		case "g":
			if m.source == nil {
				c.notice = "GitHub er ikke konfigurert. Start appen med GitHub-token for å bytte."
				return m, nil
			}
			return m.switchProvider(false)
		case "J":
			if m.jira == nil {
				c.notice = "Jira er ikke konfigurert. Start appen med JIRA_BASE_URL og JIRA_TOKEN for å bytte."
				return m, nil
			}
			return m.switchProvider(true)
		case "i":
			if c.openingIDE || c.diagnosticsOpen {
				return m, nil
			}
			var session copilot.Session
			if c.detail != nil {
				session = c.detail.Session
			} else {
				rows := c.rows()
				if c.selected >= len(rows) {
					c.ideNotice = "Velg en Copilot-sesjon først."
					return m, nil
				}
				session = rows[c.selected]
			}
			c.openingIDE = true
			c.ideNotice = "Åpner prosjektet i IntelliJ…"
			opener, ctx := c.openIDE, m.ctx
			return m, func() tea.Msg { return copilotOpenMsg{err: opener(ctx, session)} }
		case "R":
			return m, m.refreshCopilot()
		case "D":
			c.diagnosticsOpen = !c.diagnosticsOpen
			c.offset = 0
		case "/":
			c.filtering = true
			return m, c.input.Focus()
		case "esc":
			c.detail = nil
			c.diagnosticsOpen = false
			c.offset = 0
			c.input.SetValue("")
		case "j", "down":
			if c.detail != nil || c.diagnosticsOpen {
				c.offset++
			} else {
				c.selected = min(c.selected+1, max(0, len(c.rows())-1))
			}
		case "k", "up":
			if c.detail != nil || c.diagnosticsOpen {
				c.offset = max(0, c.offset-1)
			} else {
				c.selected = max(0, c.selected-1)
			}
		case "enter":
			rows := c.rows()
			if len(rows) == 0 || c.loading {
				return m, nil
			}
			s := rows[c.selected]
			c.detail = &copilot.Detail{Session: s}
			c.offset = 0
			c.diagnosticsOpen = false
			return m, m.refreshCopilot()
		}
	default:
		if c.filtering {
			var cmd tea.Cmd
			c.input, cmd = c.input.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}
func metric(n *int64) string {
	if n == nil {
		return "?"
	}
	return fmt.Sprint(*n)
}
func (m Model) copilotView() tea.View {
	c := m.copilot
	lines := []string{accent.Render(" CUSTOM VISNING · Copilot"), muted.Render(" vg GitHub · vt Jira · vs innstillinger · va Copilot · lokal historikk · egen OTLP-mottaker")}
	body := []string{}
	if c.diagnosticsOpen {
		d := c.diagnostics
		body = append(body, "Diagnostikk", "Sesjonsmappe: "+d.SessionRoot, "Eget datalager: "+d.StorePath, fmt.Sprintf("Innsamling: %t · OTLP: %t · port %d · oppdaget: %d", d.CollectionEnabled, d.ReceiverRunning, d.Port, d.Discovered), "Siste OTLP: "+d.LastOtel, fmt.Sprintf("Innhold mottatt: %t · lagring: %t · visning: %t", d.ContentReceived, d.CaptureContent, d.ShowContent))
		body = append(body, d.Errors...)
	} else if c.detail != nil {
		s := c.detail.Session
		body = append(body, s.Name+" · "+s.ID, "Status: "+s.Status+" ("+s.StatusSource+")", "Klient: "+s.Client+" · modell: "+s.Model+" · agent: "+s.Agent, "Repository: "+s.Repository+" · branch: "+s.Branch+" · Jira: "+s.Jira, "Cwd: "+s.Cwd, "Prosjekt: "+s.ProjectRoot+" · git: "+s.GitRoot, "Remote: "+s.Remote, "Start: "+s.StartedAt+" · aktivitet: "+s.LastActivity, "Tokens inn/ut: "+metric(s.InputTokens)+" / "+metric(s.OutputTokens), "Trace IDs: "+strings.Join(s.TraceIDs, ", "))
		if s.DurationMs != nil {
			body = append(body, fmt.Sprintf("Varighet: %.0f ms", *s.DurationMs))
		}
		if len(c.detail.Events) == 0 {
			body = append(body, "Ingen hendelser mottatt eller innhold er skjult.")
		}
		for _, e := range c.detail.Events {
			body = append(body, "", e.Time+" · "+e.Type+" · "+e.Source+" · "+e.Role+" "+e.Tool)
			for _, v := range []struct{ label, value string }{{"", e.Content}, {"Argumenter: ", e.Args}, {"Resultat: ", e.Result}, {"Feil: ", e.Error}, {"Advarsel: ", e.Warning}} {
				if v.value != "" {
					body = append(body, v.label+v.value)
				}
			}
		}
	} else {
		if c.filtering || c.input.Value() != "" {
			lines = append(lines, c.input.View())
		}
		rows := c.rows()
		capacity := max(1, m.height-len(lines)-4)
		start := max(0, c.selected-capacity+1)
		for i := start; i < min(len(rows), start+capacity); i++ {
			s := rows[i]
			line := cell(s.Status, 16) + " " + cell(s.Name, max(12, m.width-65)) + " " + cell(s.Repository, 22) + " " + s.Jira
			if i == c.selected {
				line = selection.Render(cell(line, m.width))
			} else {
				line = clean(line)
			}
			if m.extraSelected(copilotRef(s), "copilot") {
				line = "✓ " + line
			}
			body = append(body, line)
		}
		if len(rows) == 0 {
			body = append(body, "Ingen Copilot-sesjoner. Start en Copilot CLI-sesjon; sjekk vi for diagnostikk.")
		}
	}
	if c.detail != nil || c.diagnosticsOpen {
		wrapped := []string{}
		for _, line := range body {
			wrapped = append(wrapped, strings.Split(ansi.Hardwrap(cleanMultiline(line), max(1, m.width), true), "\n")...)
		}
		capacity := max(1, m.height-len(lines)-3)
		offset := min(c.offset, max(0, len(wrapped)-capacity))
		body = wrapped[offset:min(len(wrapped), offset+capacity)]
	}
	lines = append(lines, body...)
	if c.loading {
		lines = append(lines, muted.Render("Oppdaterer…"))
	}
	if c.ideNotice != "" {
		lines = append(lines, muted.Render(clean(c.ideNotice)))
	}
	if c.notice != "" {
		lines = append(lines, warning.Render(clean(c.notice)))
	}
	lines = append(lines, muted.Render("vs innstillinger · ee IntelliJ · d detaljer · j/k ↑/↓ · / søk · vi diagnostikk · au oppdater · Esc tilbake · Ctrl+C avslutt"))
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, max(1, m.width), "…")
	}
	if len(lines) > m.height {
		lines = append(lines[:max(0, m.height-1)], lines[len(lines)-1])
	}
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	v.WindowTitle = "custom-visning · Copilot"
	return v
}
func cleanMultiline(s string) string {
	parts := strings.Split(s, "\n")
	for i, p := range parts {
		parts[i] = clean(p)
	}
	return strings.Join(parts, "\n")
}
