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
	"github.com/haakonph/custom-visning-terminal/internal/connectors/jira"
	"github.com/haakonph/custom-visning-terminal/internal/snooze"
)

type jiraState struct {
	snoozes               *snoozeState
	client                *jira.Client
	config                jira.Config
	configPath            string
	configOK              bool
	tabs                  []jira.Query
	active, selected      int
	filters               jira.Filters
	filterBackup          jira.Filters
	input                 textinput.Model
	mode                  string
	filterIndex           int
	rows                  []jira.Issue
	query, custom, notice string
	synced                time.Time
	loading               bool
	generation            int
	cancel                context.CancelFunc
	help                  bool
}
type jiraResult struct {
	generation int
	query      string
	rows       []jira.Issue
	err        error
	at         time.Time
}

var filterNames = []string{"Project", "Status", "Assignee (username)", "Priority", "Issue type", "Sprint", "Epic key", "Label"}

func (m Model) WithJira(client *jira.Client, direct bool) Model {
	input := textinput.New()
	input.CharLimit = 8192
	path := jira.ConfigPath()
	config, err := jira.LoadConfig(path)
	state := &jiraState{snoozes: m.snoozes, client: client, config: config, configPath: path, configOK: err == nil, input: input}
	state.filters.Project = os.Getenv("JIRA_PROJECT")
	state.tabs = []jira.Query{
		{Name: "Mine", JQL: "assignee = currentUser() AND statusCategory != Done ORDER BY priority DESC, updated DESC"},
		{Name: "Prosjekt", JQL: "statusCategory != Done ORDER BY priority DESC, updated DESC"},
		{Name: "Uten ansvarlig", JQL: "assignee IS EMPTY AND statusCategory != Done ORDER BY priority DESC, updated DESC"},
		{Name: "Aktiv sprint", JQL: "assignee = currentUser() AND sprint IN openSprints() AND statusCategory != Done ORDER BY rank ASC"},
	}
	state.tabs = append(state.tabs, config.Queries...)
	if err != nil {
		state.notice = "Kunne ikke lese Jira-konfigurasjonen; rett filen før du lagrer søk"
	}
	m.jira = state
	m.jiraActive = direct
	return m
}
func (j *jiraState) currentQuery() string {
	if j.custom != "" {
		return j.custom
	}
	if j.active >= 4 {
		return j.tabs[j.active].JQL
	}
	return j.filters.JQL(j.tabs[j.active].JQL)
}
func (m *Model) refreshJira() tea.Cmd {
	j := m.jira
	query := j.currentQuery()
	if query != j.query {
		j.rows = nil
		j.synced = time.Time{}
		j.selected = 0
		j.query = query
		if cached, err := j.client.LoadCache(query); err == nil {
			j.rows = cached.Issues
			j.synced = cached.SyncedAt
		}
	}
	if j.cancel != nil {
		j.cancel()
	}
	requestCtx, cancel := context.WithCancel(m.ctx)
	j.cancel = cancel
	j.generation++
	generation := j.generation
	j.loading = true
	ctx, client := requestCtx, j.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		rows, err := client.Search(ctx, query)
		return jiraResult{generation, query, rows, err, time.Now()}
	}
}
func (j *jiraState) filtered() []jira.Issue {
	visible := make([]jira.Issue, 0, len(j.rows))
	for _, r := range j.rows {
		if !j.snoozes.hidden(snooze.Identity("jira", r.URL)) {
			visible = append(visible, r)
		}
	}
	if j.mode != "local" && j.input.Prompt != "/ " {
		return visible
	}
	term := strings.ToLower(strings.TrimSpace(j.input.Value()))
	var rows []jira.Issue
	for _, r := range visible {
		if strings.Contains(strings.ToLower(strings.Join([]string{r.Key, r.Title, r.Status, r.Priority, r.Assignee, r.Project, r.Type, strings.Join(r.Labels, " ")}, " ")), term) {
			rows = append(rows, r)
		}
	}
	return rows
}
func (j *jiraState) clamp() { j.selected = max(0, min(j.selected, len(j.filtered())-1)) }
func (j *jiraState) edit(mode, prompt, value string) tea.Cmd {
	j.mode = mode
	j.input.Prompt = prompt
	j.input.SetValue(value)
	j.input.CursorEnd()
	return j.input.Focus()
}
func (j *jiraState) filterPointers() []*string {
	return []*string{&j.filters.Project, &j.filters.Status, &j.filters.Assignee, &j.filters.Priority, &j.filters.Type, &j.filters.Sprint, &j.filters.Epic, &j.filters.Label}
}
func (m Model) updateJira(msg tea.Msg) (tea.Model, tea.Cmd) {
	j := m.jira
	switch msg := msg.(type) {
	case openMsg:
		if msg.err != nil {
			j.notice = "Kunne ikke åpne Jira i nettleseren"
		}
	case jiraResult:
		if msg.generation != j.generation {
			return m, nil
		}
		j.loading = false
		if msg.err != nil {
			j.notice = msg.err.Error()
			return m, nil
		}
		j.rows = msg.rows
		j.synced = msg.at
		j.notice = ""
		j.clamp()
		if err := j.client.SaveCache(jira.Cached{Query: msg.query, Issues: msg.rows, SyncedAt: msg.at}); err != nil {
			j.notice = "Kunne ikke lagre Jira-cache"
		}
	case tea.KeyPressMsg:
		key := msg.String()
		if key == "ctrl+c" {
			m.cancel()
			return m, tea.Quit
		}
		if j.mode != "" {
			if key == "esc" {
				if j.mode == "filter" {
					j.filters = j.filterBackup
				}
				j.mode = ""
				j.input.Blur()
				j.input.SetValue("")
				j.input.Prompt = ""
				return m, nil
			}
			if key == "enter" {
				value := strings.TrimSpace(j.input.Value())
				mode := j.mode
				j.mode = ""
				j.input.Blur()
				j.input.Prompt = ""
				switch mode {
				case "local":
					j.input.Prompt = "/ "
					j.clamp()
					return m, nil
				case "jql":
					if value == "" {
						return m, nil
					}
					j.custom = value
					return m, m.refreshJira()
				case "name":
					if value == "" {
						return m, nil
					}
					if !j.configOK {
						j.notice = "Rett Jira-konfigurasjonsfilen før du lagrer"
						return m, nil
					}
					config := jira.Config{Queries: append([]jira.Query(nil), j.config.Queries...)}
					q := jira.Query{Name: value, JQL: j.currentQuery()}
					found := false
					for i := range config.Queries {
						if config.Queries[i].Name == value {
							config.Queries[i] = q
							found = true
							break
						}
					}
					if !found {
						config.Queries = append(config.Queries, q)
					}
					if err := jira.SaveConfig(j.configPath, config); err != nil {
						j.notice = "Kunne ikke lagre søket"
						return m, nil
					}
					j.config = config
					j.tabs = append(j.tabs[:4:4], config.Queries...)
					j.notice = "Søk lagret: " + value
				case "filter":
					*j.filterPointers()[j.filterIndex] = value
					j.filterIndex++
					if j.filterIndex < len(filterNames) {
						return m, j.edit("filter", filterNames[j.filterIndex]+": ", *j.filterPointers()[j.filterIndex])
					}
					j.custom = ""
					return m, m.refreshJira()
				}
				return m, nil
			}
			var cmd tea.Cmd
			j.input, cmd = j.input.Update(msg)
			j.selected = 0
			return m, cmd
		}
		switch key {
		case "?":
			j.help = !j.help
		case "q":
			m.cancel()
			return m, tea.Quit

		case "j", "down":
			j.selected++
			j.clamp()
		case "k", "up":
			j.selected--
			j.clamp()
		case "tab", "right", "shift+tab", "left":
			delta := 1
			if key == "shift+tab" || key == "left" {
				delta = -1
			}
			j.active = (j.active + delta + len(j.tabs)) % len(j.tabs)
			j.custom = ""
			j.selected = 0
			return m, m.refreshJira()
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			index := int(key[0] - '1')
			if index >= len(j.tabs) {
				return m, nil
			}
			j.active = index
			j.custom = ""
			j.selected = 0
			return m, m.refreshJira()
		case "r":
			return m, m.refreshJira()
		case "/":
			return m, j.edit("local", "/ ", "")
		case ":":
			return m, j.edit("jql", "JQL: ", j.currentQuery())
		case "s":
			return m, j.edit("name", "Lagre søk som: ", "")
		case "f":
			if j.active >= 4 || j.custom != "" {
				j.notice = "Hurtigfiltre gjelder standardfanene; bruk : for å endre lagret JQL"
				return m, nil
			}
			j.filterBackup = j.filters
			j.filterIndex = 0
			return m, j.edit("filter", filterNames[0]+": ", j.filters.Project)
		case "esc":
			j.input.SetValue("")
			j.input.Prompt = ""
			j.selected = 0
		case "x":
			j.filters = jira.Filters{}
			j.custom = ""
			return m, m.refreshJira()
		case "enter":
			rows := j.filtered()
			if len(rows) == 0 {
				return m, nil
			}
			link := rows[j.selected].URL
			return m, func() tea.Msg { return openMsg{err: openJiraBrowser(m.ctx, j.client, link)} }
		}
	default:
		if j.mode != "" {
			var cmd tea.Cmd
			j.input, cmd = j.input.Update(msg)
			j.selected = 0
			return m, cmd
		}
	}
	return m, nil
}
func (m Model) jiraView() tea.View {
	j := m.jira
	lines := []string{accent.Render(" CUSTOM VISNING · JIRA") + muted.Render("  g GitHub · [J Jira] · c Copilot")}
	var tabs []string
	for i, t := range j.tabs {
		label := t.Name
		if i < 9 {
			label = fmt.Sprintf("%d %s", i+1, t.Name)
		}
		if i == j.active {
			label = "[" + label + "]"
		}
		tabs = append(tabs, label)
	}
	lines = append(lines, strings.Join(tabs, "  "))
	lines = append(lines, m.snoozeLines()...)
	sync := "aldri"
	if !j.synced.IsZero() {
		sync = j.synced.Local().Format("2006-01-02 15:04:05")
	}
	state := "klar"
	if j.loading {
		state = "henter…"
	}
	lines = append(lines, fmt.Sprintf(" %s · siste synk: %s", state, sync), " JQL: "+clean(j.query))
	if j.help {
		lines = append(lines, "g: GitHub · J (Shift+j): Jira (i navigasjonsmodus)", "1–9: velg fane direkte · Tab / Shift+Tab: neste / forrige fane", "f: kombiner filtre, Enter neste felt, Esc avbryter", ": skriv JQL (inkludert ORDER BY); s: lagre med navn", "/: filtrer lokalt; x: nullstill hurtigfiltre og egen JQL", "JQL-søk kjøres i Jira. Lagrede søk brukes uten hurtigfiltre.")
	}
	if j.mode != "" || j.input.Prompt == "/ " {
		lines = append(lines, j.input.View())
	}
	if j.mode == "filter" {
		lines = append(lines, " Enter neste felt · tomt felt fjerner filter · Esc avbryt · søket kjøres etter siste felt")
	}
	rows := j.filtered()
	selected := max(0, min(j.selected, len(rows)-1))
	row := func(r jira.Issue, header bool) string {
		values := []string{r.Key, r.Status, r.Title}
		widths := []int{13, 16, max(1, m.width-31)}
		if header {
			values = []string{"SAK", "STATUS", "TITTEL"}
		}
		if m.width >= 100 {
			values = []string{r.Key, r.Priority, r.Status, r.Title, r.Assignee, age(r.Updated)}
			widths = []int{13, 9, 16, max(1, m.width-65), 16, 6}
			if header {
				values = []string{"SAK", "PRIORITET", "STATUS", "TITTEL", "ANSVARLIG", "ENDRET"}
			}
		}
		if m.width < 45 {
			values = []string{r.Key, r.Title}
			widths = []int{12, max(1, m.width-13)}
			if header {
				values = []string{"SAK", "TITTEL"}
			}
		}
		var cells []string
		for i, v := range values {
			cells = append(cells, cell(v, widths[i]))
		}
		return strings.Join(cells, " ")
	}
	lines = append(lines, accent.Render(row(jira.Issue{}, true)))
	capacity := max(1, m.height-len(lines)-6)
	start := max(0, selected-capacity+1)
	for i := start; i < min(len(rows), start+capacity); i++ {
		line := row(rows[i], false)
		if i == selected {
			line = selection.Render(line)
		}
		lines = append(lines, line)
	}
	if len(rows) == 0 {
		lines = append(lines, " Ingen saker i dette søket.")
	}
	if len(rows) > 0 {
		r := rows[selected]
		lines = append(lines, clean(fmt.Sprintf(" %d/%d · %s · %s · %s · frist: %s · labels: %s", selected+1, len(rows), r.Project, r.Type, r.Assignee, r.Due, strings.Join(r.Labels, ", "))), clean(" "+r.Description))
	}
	if j.notice != "" {
		lines = append(lines, warning.Render(clean(j.notice)))
	}
	lines = append(lines, "T oppgaver · t ny fra valgt · a legg til · l relaterte PR-er · g GitHub · J Jira · c Copilot · 1–9 / Tab faner · j/k velg · Enter åpne · / lokalt søk · f filtre · : JQL · s lagre · x nullstill · r hent · ? hjelp · q avslutt")
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], m.width, "…")
	}
	if len(lines) > m.height {
		lines = append(lines[:max(0, m.height-1)], lines[len(lines)-1])
	}
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	view.WindowTitle = "custom-visning · Jira"
	return view
}
