package tui

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/local"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

type writableRepositorySource interface {
	WritableRepositories(context.Context) ([]string, error)
	CheckWritable(context.Context, string) error
}

type folderState struct {
	active, loading, searching, busy bool
	repoLoading                      bool
	rows                             []local.Folder
	selected, tab                    int
	query, notice                    string
	picking                          bool
	openAfterCreate                  bool
	repositoryView                   bool
	repositories                     []string
	issue                            model.TaskReference
	generation                       uint64
}
type foldersLoaded struct {
	rows []local.Folder
	err  error
}
type folderReposLoaded struct {
	rows       []string
	err        error
	generation uint64
}
type folderCreated struct {
	openRequested bool
	openErr       error
	folder        local.Folder
	err           error
}
type folderOpened struct{ err error }
type repositoryOpened struct{ err error }

func (m *Model) loadWritableRepositories() tea.Cmd {
	if m.folders.repoLoading {
		return nil
	}
	source, ok := m.source.(writableRepositorySource)
	if !ok {
		m.folders.notice = "GitHub må være konfigurert. Åpne vg og vh for oppsett."
		return nil
	}
	m.folders.generation++
	generation, parent := m.folders.generation, m.ctx
	m.folders.repoLoading = true
	m.folders.notice = "Henter repoer med skrivetilgang…"
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
		defer cancel()
		rows, err := source.WritableRepositories(ctx)
		return folderReposLoaded{rows, err, generation}
	}
}

func (m Model) openWritableRepository() (tea.Model, tea.Cmd) {
	rows := m.folderRepositories()
	if len(rows) == 0 || m.folders.repoLoading {
		return m, nil
	}
	id := rows[min(m.folders.selected, len(rows)-1)]
	owner, name, ok := strings.Cut(id, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return m, nil
	}
	link, parent := "https://github.com/"+url.PathEscape(owner)+"/"+url.PathEscape(name), m.ctx
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 10*time.Second)
		defer cancel()
		return repositoryOpened{openBrowser(ctx, link)}
	}
}

func (m *Model) loadFolders() tea.Cmd {
	if m.local == nil || m.folders.loading {
		return nil
	}
	m.folders.loading = true
	manager, parent := m.local.manager, m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 30*time.Second)
		defer cancel()
		rows, err := manager.Folders(ctx)
		return foldersLoaded{rows, err}
	}
}
func (m Model) folderRows() []local.Folder {
	var rows []local.Folder
	for _, f := range m.folders.rows {
		if m.folders.tab == 2 {
			if !f.Hidden {
				continue
			}
		} else if f.Hidden {
			continue
		}
		if m.folders.tab == 1 && !f.Favorite {
			continue
		}
		if !strings.Contains(strings.ToLower(f.Path+" "+f.Repository+" "+f.IssueKey+" "+f.Title), strings.ToLower(m.folders.query)) {
			continue
		}
		rows = append(rows, f)
	}
	return rows
}
func (m *Model) startIssueFolder(refs []model.TaskReference) tea.Cmd {
	if m.folders.busy || m.folders.picking {
		return nil
	}
	if m.local == nil {
		m.sequenceNotice = "Sett opp lokale repositories og editor i vs."
		return nil
	}
	if len(refs) != 1 || refs[0].Provider != "jira" {
		m.sequenceNotice = "Velg én Jira-oppgave for å opprette en mappe."
		return nil
	}
	_, ok := m.source.(writableRepositorySource)
	if !ok {
		m.sequenceNotice = "GitHub må være konfigurert for å hente repoer med skrivetilgang."
		return nil
	}
	m.folders.openAfterCreate = false
	m.folders.repositoryView = false
	m.folders.picking = true
	m.folders.searching = false
	m.folders.issue = refs[0]
	m.folders.repositories = nil
	m.folders.selected = 0
	m.folders.query = ""
	return m.loadWritableRepositories()
}
func (m *Model) folderMessage(msg tea.Msg) bool {
	switch r := msg.(type) {
	case foldersLoaded:
		m.folders.loading = false
		if r.err != nil {
			m.folders.notice = r.err.Error()
		} else {
			m.folders.rows = r.rows
			m.folders.notice = ""
		}
		m.folders.selected = max(0, min(m.folders.selected, len(m.folderRows())-1))
	case folderReposLoaded:
		if r.generation != m.folders.generation || (!m.folders.picking && !m.folders.repositoryView) {
			return true
		}
		m.folders.repoLoading = false
		if r.err == nil {
			m.folders.repositories = r.rows
		}
		m.folders.selected = max(0, min(m.folders.selected, len(m.folderRepositories())-1))
		m.folders.notice = "Velg repository og trykk Enter for å opprette mappen."
		if m.folders.openAfterCreate {
			m.folders.notice = "Velg repository og trykk Enter for å åpne arbeidsmappen i editor."
		}
		if m.folders.repositoryView {
			m.folders.notice = "Viser aktive repoer tokenet gir skrivetilgang til. Arkiverte og deaktiverte repoer utelates."
		}
		if r.err != nil {
			m.folders.notice = r.err.Error()
		} else if len(r.rows) == 0 {
			m.folders.notice = "Ingen repoer med skrivetilgang. Kontroller GitHub-token og SSO."
		}
	case folderCreated:
		m.folders.busy = false
		if r.err != nil {
			m.folders.notice = r.err.Error()
			return true
		}
		m.folders.picking = false
		m.folders.active = true
		found := false
		for i, f := range m.folders.rows {
			if f.Path == r.folder.Path {
				m.folders.rows[i] = r.folder
				found = true
			}
		}
		if !found {
			m.folders.rows = append(m.folders.rows, r.folder)
		}
		m.folders.selected = 0
		m.folders.tab = 0
		if r.folder.Hidden {
			m.folders.tab = 2
		}
		m.folders.query = r.folder.Path
		m.folders.notice = "Mappe klar: " + r.folder.Path + " · ee åpner editor"
		if r.openRequested {
			m.folders.notice = "Mappe åpnet i editor: " + r.folder.Path
			if r.openErr != nil {
				m.folders.notice = "Mappe klar, men editor kunne ikke åpnes: " + r.openErr.Error() + " · ee prøver igjen"
			}
		}
	case repositoryOpened:
		m.folders.notice = "Repository åpnet på GitHub."
		if r.err != nil {
			m.folders.notice = "Kunne ikke åpne nettleseren: " + r.err.Error()
		}
	case folderOpened:
		m.folders.busy = false
		m.folders.notice = "Mappen er åpnet i editor."
		if r.err != nil {
			m.folders.notice = r.err.Error()
		}
	default:
		return false
	}
	return true
}
func (m Model) folderRepositories() []string {
	var rows []string
	for _, r := range m.folders.repositories {
		if strings.Contains(strings.ToLower(r), strings.ToLower(m.folders.query)) {
			rows = append(rows, r)
		}
	}
	return rows
}
func (m Model) folderKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	f := &m.folders
	k := key.String()
	if f.picking && !f.searching && (k != "/" || f.query != "") {
		if pickerQuery(key, &f.query) {
			f.selected = 0
			return m, nil
		}
	}
	if f.searching {
		switch k {
		case "enter":
			f.searching = false
		case "esc":
			f.searching = false
			f.query = ""
		case "backspace":
			r := []rune(f.query)
			if len(r) > 0 {
				f.query = string(r[:len(r)-1])
			}
		default:
			f.query += key.Text
		}
		f.selected = 0
		return m, nil
	}
	count := len(m.folderRows())
	if f.picking || f.repositoryView {
		count = len(m.folderRepositories())
	}
	switch k {
	case "esc":
		if f.busy {
			f.notice = "Vent til opprettelsen er ferdig."
			return m, nil
		}
		if f.repositoryView {
			f.repositoryView, f.active, f.repoLoading = false, false, false
			f.query = ""
			f.generation++
		} else if f.picking {
			f.picking = false
			f.repoLoading = false
			f.generation++
			f.query = ""
		} else {
			f.active = false
		}
	case "j", "down":
		f.selected = min(f.selected+1, max(0, count-1))
	case "k", "up":
		f.selected = max(0, f.selected-1)
	case "h", "left", "l", "right":
		if !f.picking && !f.repositoryView {
			delta := 1
			if k == "h" || k == "left" {
				delta = 2
			}
			f.tab = (f.tab + delta) % 3
			f.selected = 0
		}
	case "/":
		f.searching = true
	case "enter":
		if f.repositoryView {
			return m.openWritableRepository()
		}
		if f.picking && !f.repoLoading && !f.busy && count > 0 {
			source, ok := m.source.(writableRepositorySource)
			if !ok {
				return m, nil
			}
			id, issue := m.folderRepositories()[f.selected], f.issue
			manager, parent := m.local.manager, m.ctx
			launcher, openRequested := m.local.launcher, f.openAfterCreate
			editor := launcher.Editors.Default
			if editor == "" {
				editor = "intellij"
			}
			f.busy = true
			f.notice = "Oppretter mappe for " + issue.Key + "…"
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(parent, 20*time.Minute)
				defer cancel()
				if err := source.CheckWritable(ctx, id); err != nil {
					return folderCreated{err: err}
				}
				folder, err := manager.CreateIssue(ctx, id, issue.Key, issue.Title)
				result := folderCreated{folder: folder, err: err, openRequested: openRequested}
				if err == nil && openRequested {
					result.openErr = launcher.Launch(editor, folder.Path)
				}
				return result
			}
		}
	}
	return m, nil
}
func (m Model) folderAction(prefix, k string) (tea.Model, tea.Cmd) {
	if m.folders.repositoryView {
		if prefix == "a" && k == "u" {
			return m, m.loadWritableRepositories()
		}
		if prefix == "e" && k == "n" {
			return m.openWritableRepository()
		}
		return m, nil
	}
	if m.folders.picking {
		return m, nil
	}
	if prefix == "a" && k == "u" {
		return m, m.loadFolders()
	}
	rows := m.folderRows()
	if len(rows) == 0 {
		return m, nil
	}
	folder := rows[min(m.folders.selected, len(rows)-1)]
	if prefix == "e" && k == "e" && !m.folders.busy {
		launcher := m.local.launcher
		editor := launcher.Editors.Default
		if editor == "" {
			editor = "intellij"
		}
		m.folders.busy = true
		return m, func() tea.Msg {
			info, err := os.Stat(folder.Path)
			if err == nil && !info.IsDir() {
				err = fmt.Errorf("mappen er ikke en katalog")
			}
			if err == nil {
				err = launcher.Launch(editor, folder.Path)
			}
			return folderOpened{err}
		}
	}
	if prefix == "a" && (k == "f" || k == "h") {
		if k == "f" {
			folder.Favorite = !folder.Favorite
		} else {
			folder.Hidden = !folder.Hidden
		}
		if m.localBusy() {
			return m, nil
		}
		manager, parent := m.local.manager, m.ctx
		var rows []local.Folder
		m.folders.notice = "Lagrer…"
		m.enqueueLocal(func() error {
			if err := manager.SaveFolder(folder); err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(parent, 30*time.Second)
			defer cancel()
			var err error
			rows, err = manager.Folders(ctx)
			return err
		}, func(m *Model, err error) {
			if err != nil {
				m.folders.notice = err.Error()
				return
			}
			m.folders.rows = rows
			m.folders.selected = max(0, min(m.folders.selected, len(m.folderRows())-1))
			m.folders.notice = "Lagret"
		})
		return m, nil
	}
	return m, nil
}
func (m Model) foldersView() tea.View {
	f := m.folders
	title := "Lokale mapper · Alle / Favoritter / Skjulte"
	var rows []string
	if f.picking || f.repositoryView {
		title = "Ny mappe fra " + f.issue.Key + " · velg repository med skrivetilgang"
		for _, id := range m.folderRepositories() {
			rows = append(rows, m.githubName(id)+"  ·  "+id)
		}
	} else {
		title = "Lokale mapper · " + []string{"[Alle] Favoritter Skjulte", "Alle [Favoritter] Skjulte", "Alle Favoritter [Skjulte]"}[f.tab]
		for _, folder := range m.folderRows() {
			mark := "  "
			if folder.Favorite {
				mark = "★ "
			}
			rows = append(rows, folderTableRow(m.width-2, mark, folder.Repository, folder.IssueKey, folder.Title, folder.Path))
		}
	}
	if f.repositoryView {
		title = fmt.Sprintf("Repoer med skrivetilgang (vp) · %d av %d", len(rows), len(f.repositories))
	}
	lines := []string{accent.Render(cell(title, m.width))}
	loading := f.loading
	if f.picking || f.repositoryView {
		loading = f.repoLoading
	}
	if f.query != "" || f.searching {
		lines = append(lines, "/ "+clean(f.query))
	}
	if loading {
		lines = append(lines, "Laster…")
	}
	if !f.picking && !f.repositoryView {
		lines = append(lines, accent.Render("  "+folderTableRow(m.width-2, "★", "REPOSITORY", "JIRA", "TITTEL", "STI")))
	}
	if len(rows) == 0 && !loading {
		lines = append(lines, "Ingen mapper eller repoer i dette utvalget.")
	}
	space := max(1, m.height-len(lines)-3)
	start := max(0, f.selected-space+1)
	for i := start; i < min(len(rows), start+space); i++ {
		line := cell(rows[i], max(1, m.width-2))
		if i == f.selected {
			line = selection.Render("> " + line)
		} else {
			line = "  " + line
		}
		lines = append(lines, line)
	}
	if f.notice != "" {
		lines = append(lines, cell(f.notice, m.width))
	}
	hint := "h/l faner · j/k velg · / søk · af favoritt · ah skjul/vis · ee editor · au oppdater · Esc tilbake"
	if f.picking || f.repositoryView {
		hint = "j/k velg · / søk · Enter opprett · Esc avbryt"
		if f.openAfterCreate {
			hint = "j/k velg · / søk · Enter åpne i editor · Esc avbryt"
		}
		if f.picking {
			hint = strings.Replace(hint, "j/k velg · / søk", "Skriv for å søke · ↑/↓ velg", 1)
		}
	}
	if f.repositoryView {
		hint = "j/k velg · / søk · Enter/en GitHub · au oppdater · Esc tilbake"
	}
	lines = append(lines, muted.Render(cell(hint, m.width)))
	if len(lines) > max(1, m.height) {
		lines = lines[:max(1, m.height)]
	}
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	return view
}

// Keep headers and values aligned, reserving room for the path in narrow terminals.
func folderTableRow(width int, favorite, repository, issue, title, path string) string {
	width = max(1, width)
	values := []string{favorite, repository, issue, title, path}
	widths := []int{2, 22, 12, 24, 1}
	if width < 100 {
		widths = []int{2, 16, 10, 16, 1}
	}
	if width < 70 {
		values = []string{favorite, repository, path}
		widths = []int{2, min(16, width/3), 1}
	}
	used := len(widths) - 1
	for _, size := range widths[:len(widths)-1] {
		used += size
	}
	widths[len(widths)-1] = max(1, width-used)
	for i := range values {
		values[i] = cell(values[i], widths[i])
	}
	return cell(strings.Join(values, " "), width)
}
