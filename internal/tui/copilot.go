package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/haakonph/custom-visning-terminal/internal/copilot"
	"github.com/haakonph/custom-visning-terminal/internal/syncstate"
)

type copilotState struct {
	formattedDetail *copilot.Detail
	formattedWidth  int
	formattedLines  []string

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
	token       syncstate.Token
	sessions    []copilot.Session
	detail      *copilot.Detail
	diagnostics copilot.Diagnostics
	id          string
	err         error
}
type copilotTick struct{}
type copilotOpenMsg struct {
	err   error
	token syncstate.Token
}

func (m Model) WithCopilot(client *copilot.Service, active bool) Model {
	m.copilotSession.Configure(m.ctx)
	input := textinput.New()
	input.Prompt = "/ "
	input.Placeholder = "session, repository, Jira, model, status"
	input.CharLimit = 256
	m.copilot = &copilotState{entities: m.entities, snoozes: m.snoozes, client: client, input: input, openIDE: openIntelliJ}
	m.setPageFlag(pageCopilot, active)
	if active {
		m.setPageFlag(pageJira, false)
		m.booting = false
	}
	return m
}
func (m Model) switchCopilot() (tea.Model, tea.Cmd) {
	if m.copilot == nil {
		m.notice = "Copilot is not configured"
		return m, nil
	}
	m.setPageFlag(pageCopilot, true)
	m.setPageFlag(pageJira, false)
	return m, m.refreshCopilot()
}
func (m *Model) refreshCopilot() tea.Cmd {
	c := m.copilot
	if c == nil || c.client == nil || c.loading {
		return nil
	}
	c.loading = true
	client, ctx := c.client, m.copilotSession.Context(m.ctx)
	token := m.copilotSession.Begin("refresh")
	id := ""
	if c.detail != nil && m.visibleProvider() == "copilot" {
		id = c.detail.Session.ID
	}
	return func() tea.Msg {
		sessions, err := client.Sessions(ctx)
		result := copilotResult{token: token, sessions: sessions, id: id, err: err}
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
		if msg.token.Sequence != 0 && !m.copilotSession.Match("open", msg.token) {
			return m, nil
		}
		c.openingIDE = false
		if msg.err != nil {
			c.ideNotice = msg.err.Error()
		} else {
			c.ideNotice = "Åpneforespørsel sendt til IntelliJ for sesjonens prosjekt."
		}
	case copilotTick:
		c.timerPending = false
		return m, m.refreshCopilot()
	case copilotResult:
		if msg.token.Sequence != 0 && !m.copilotSession.Accept("refresh", msg.token) {
			return m, nil
		}
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
		if c.client != nil && !c.timerPending {
			c.timerPending = true
			interval := time.Minute
			if m.visibleProvider() == "copilot" && !m.settings.active && !m.help {
				interval = 3 * time.Second
			}
			return m, tea.Tick(interval, func(time.Time) tea.Msg { return copilotTick{} })
		}
	case tea.KeyPressMsg:
		key := msg.String()
		if key == "ctrl+c" {
			return m, m.quit()
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
			return m, m.quit()
		case "g":
			if m.prs.Source() == nil {
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
			opener, ctx := c.openIDE, m.copilotSession.Context(m.ctx)
			token := m.copilotSession.Begin("open")
			return m, func() tea.Msg { return copilotOpenMsg{err: opener(ctx, session), token: token} }
		case "R":
			return m, m.refreshCopilot()
		case "D":
			c.diagnosticsOpen = !c.diagnosticsOpen
			c.offset = 0
		case "/":
			return m.searchCopilot()

		case "esc":
			return m.backCopilot()

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
			return m.openCopilotDetails()

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
func (c *copilotState) otlpExporterConfig() string {
	return fmt.Sprintf("export OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:%d\nexport OTEL_EXPORTER_OTLP_PROTOCOL=http/json", c.diagnostics.Port)
}

func (m Model) copilotView() tea.View {
	c := m.copilot
	lines := []string{accent.Render(" CUSTOM VISNING · Copilot"), muted.Render(" vg GitHub · vt Jira · vs innstillinger · va Copilot · lokal historikk · egen OTLP-mottaker")}
	body := []string{}
	if c.diagnosticsOpen {
		d := c.diagnostics
		body = append(body, "Diagnostikk", "Sesjonsmappe: "+d.SessionRoot, "Eget datalager: "+d.StorePath, fmt.Sprintf("Innsamling: %t · OTLP: %t · port %d · oppdaget: %d", d.CollectionEnabled, d.ReceiverRunning, d.Port, d.Discovered), "Siste OTLP: "+d.LastOtel, fmt.Sprintf("Innhold mottatt: %t · lagring: %t · visning: %t", d.ContentReceived, d.CaptureContent, d.ShowContent))
		body = append(body,
			fmt.Sprintf("OTLP localhost-URL: http://127.0.0.1:%d", d.Port),
			"OTLP-protokoller: http/json og http/protobuf støttes · grpc støttes ikke",
			"HTTP-endepunkter: /v1/traces · /v1/logs · /v1/metrics",
			"y kopierer URL og protokoll som eksportinnstillinger:",
			c.otlpExporterConfig(),
			"Alternativ protokoll: OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf",
		)
		body = append(body, d.Errors...)
	} else if c.detail != nil {
		body = m.copilotDetailLines()
	} else {
		if c.filtering || c.input.Value() != "" {
			lines = append(lines, c.input.View())
		}
		lines = append(lines, accent.Render(m.copilotListRow("STATUS", "SESJON", "REPOSITORY", "JIRA")))
		rows := c.rows()
		capacity := max(1, m.height-len(lines)-4)
		start := max(0, c.selected-capacity+1)
		for i := start; i < min(len(rows), start+capacity); i++ {
			s := rows[i]
			line := m.copilotListRow(s.Status, s.Name, m.githubName(s.Repository), s.Jira)
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
			body = append(body, "Ingen Copilot-sesjoner. vh viser oppsett; vi viser diagnostikk.", "Start en Copilot CLI-sesjon. Historikk leses fra ~/.copilot/session-state.", "vs lar deg endre sesjonsmappe og innsamling; au skanner på nytt.")
		}
	}
	if c.diagnosticsOpen {
		wrapped := []string{}
		for _, line := range body {
			wrapped = append(wrapped, strings.Split(ansi.Hardwrap(cleanMultiline(line), max(1, m.width), true), "\n")...)
		}
		capacity := max(1, m.height-len(lines)-3)
		offset := min(c.offset, max(0, len(wrapped)-capacity))
		body = wrapped[offset:min(len(wrapped), offset+capacity)]
	}
	if c.detail != nil && !c.diagnosticsOpen {
		capacity := max(1, m.height-len(lines)-3)
		offset := min(c.offset, max(0, len(body)-capacity))
		body = body[offset:min(len(body), offset+capacity)]
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
	lines = append(lines, muted.Render("vs innstillinger · eee chatmappe · eew worktree (IntelliJ) · d detaljer · j/k ↑/↓ · / søk · vi diagnostikk · au oppdater · Esc tilbake · Ctrl+C avslutt"))
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

func (m Model) copilotListRow(status, name, repository, jira string) string {
	return cell(status, 16) + " " + cell(name, max(12, m.width-65)) + " " + cell(repository, 22) + " " + jira
}

func cleanMultiline(s string) string {
	parts := strings.Split(s, "\n")
	for i, p := range parts {
		parts[i] = clean(p)
	}
	return strings.Join(parts, "\n")
}

func (m Model) copilotDetailLines() []string {
	c := m.copilot
	if c.formattedDetail == c.detail && c.formattedWidth == m.width {
		return c.formattedLines
	}
	var body []string
	s := c.detail.Session
	body = append(body, s.Name+" · "+s.ID, "Status: "+s.Status+" ("+s.StatusSource+")", "Klient: "+s.Client+" · modell: "+s.Model+" · agent: "+s.Agent, "Repository: "+m.githubName(s.Repository)+" · branch: "+s.Branch+" · Jira: "+s.Jira, "Cwd: "+s.Cwd, "Prosjekt: "+s.ProjectRoot+" · git: "+s.GitRoot, "Remote: "+s.Remote, "Start: "+s.StartedAt+" · aktivitet: "+s.LastActivity, "Tokens inn/ut: "+metric(s.InputTokens)+" / "+metric(s.OutputTokens), "Trace IDs: "+strings.Join(s.TraceIDs, ", "))
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
	var wrapped []string
	for _, line := range body {
		wrapped = append(wrapped, strings.Split(ansi.Hardwrap(cleanMultiline(line), max(1, m.width), true), "\n")...)
	}
	c.formattedDetail, c.formattedWidth, c.formattedLines = c.detail, m.width, wrapped
	return wrapped
}

func (m Model) searchCopilot() (tea.Model, tea.Cmd) {
	c := m.copilot

	c.filtering = true
	return m, c.input.Focus()
}

func (m Model) backCopilot() (tea.Model, tea.Cmd) {
	c := m.copilot

	c.detail = nil
	c.diagnosticsOpen = false
	c.offset = 0
	c.input.SetValue("")
	return m, nil
}

func (m Model) openCopilotDetails() (tea.Model, tea.Cmd) {
	c := m.copilot

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
