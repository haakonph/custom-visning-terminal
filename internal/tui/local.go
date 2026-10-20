package tui

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/local"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

type localState struct {
	statusLoading, statusAgain bool
	generation                 uint64
	manager                    *local.WorktreeManager
	launcher                   local.EditorLauncher
	chosen                     map[string]model.PullRequest
	statuses                   map[string]local.Status
	preparing                  map[string]bool
	updating                   map[string]bool
	queue                      []model.PullRequest
	running                    int
	editor                     string
	choosing                   bool
	editorChoice               int
	results                    []string
	warmed                     map[string]time.Time
	warming                    bool
	warmNotice                 string
	warmTimerStarted           bool
	cleanupRunning             bool
	cleanupAt                  time.Time
}
type localCleanupResult struct{ err error }
type localWarmTick struct{}

func localWarmTimer() tea.Cmd {
	return tea.Tick(30*time.Second, func(time.Time) tea.Msg { return localWarmTick{} })
}

type localWarmResult struct {
	repository string
	err        error
}
type localResult struct {
	p       model.PullRequest
	status  local.Status
	err     error
	updated bool
}
type localOpened struct {
	p    model.PullRequest
	path string
}
type localStatuses struct {
	values     map[string]local.Status
	generation uint64
}

func (m Model) WithLocal(manager *local.WorktreeManager, editors local.Editors) Model {
	m.local = &localState{manager: manager, launcher: local.EditorLauncher{Editors: editors}, chosen: m.chosen, statuses: map[string]local.Status{}, preparing: map[string]bool{}, updating: map[string]bool{}, warmed: map[string]time.Time{}}
	return m
}
func (m *Model) refreshLocal() tea.Cmd {
	if m.local == nil {
		return nil
	}
	if m.local.statusLoading {
		m.local.statusAgain = true
		return nil
	}
	m.local.statusLoading = true
	generation := m.local.generation
	rows := []model.PullRequest{}
	seen := map[string]bool{}
	for _, d := range m.snapshot.Datasets {
		for _, p := range d.PRs {
			if !seen[p.Key()] {
				rows = append(rows, p)
				seen[p.Key()] = true
			}
		}
	}
	manager, parent := m.local.manager, m.ctx
	status := func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
		defer cancel()
		result := localStatuses{values: map[string]local.Status{}, generation: generation}
		for _, p := range rows {
			if ctx.Err() != nil {
				break
			}
			result.values[p.Key()] = manager.Status(ctx, p)
		}
		return result
	}
	var timer tea.Cmd
	if !m.local.warmTimerStarted {
		m.local.warmTimerStarted = true
		timer = localWarmTimer()
	}
	return tea.Batch(status, m.warmLocal(), m.cleanupIntelliJ(true), timer)
}

func (m *Model) cleanupIntelliJ(force bool) tea.Cmd {
	l := m.local
	source, ok := m.source.(local.PRClosureSource)
	if !ok || l.cleanupRunning || (!force && time.Since(l.cleanupAt) < 5*time.Minute) {
		return nil
	}
	l.cleanupRunning = true
	manager, parent := l.manager, m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
		defer cancel()
		return localCleanupResult{manager.CleanupIntelliJ(ctx, source)}
	}
}

func (m *Model) warmLocal() tea.Cmd {
	l := m.local
	if !l.manager.Workspace.Warm || l.warming {
		return nil
	}
	var id string
	var oldest time.Time
	for _, dataset := range m.snapshot.Datasets {
		for _, p := range dataset.PRs {
			last := l.warmed[p.Repository]
			if p.Repository != "" && time.Since(last) >= 5*time.Minute && (id == "" || last.Before(oldest)) {
				id, oldest = p.Repository, last
			}
		}
	}
	if id == "" {
		return nil
	}
	l.warmed[id], l.warming = time.Now(), true
	l.warmNotice = "Klargjør main: " + m.githubName(id)
	manager, parent := l.manager, m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 15*time.Minute)
		defer cancel()
		return localWarmResult{id, manager.Warm(ctx, id)}
	}
}

func (m *Model) openLocal(editor string) tea.Cmd {
	if m.local == nil {
		*m = m.WithSetup("editor")
		return nil
	}
	l := m.local
	if l.running > 0 || len(l.preparing) > 0 {
		m.notice = "An open operation is already running"
		return nil
	}
	// Validate the editor before any clone or fetch.
	if _, err := l.launcher.Command(editor, "."); err != nil {
		*m = m.WithSetupProblem("editor", err.Error())
		return nil
	}
	rows := m.selectedPRs()
	if len(rows) == 0 {
		return nil
	}
	l.generation++
	l.queue = rows
	l.editor = editor
	l.results = nil
	for _, p := range rows {
		l.preparing[p.Key()] = true
	}
	m.notice = fmt.Sprintf("Preparing %d PR worktrees…", len(rows))
	return m.nextLocal()
}

// At most three opening jobs are scheduled. Existing worktrees release their
// opening slot before updates, which the repository manager serializes per repo.
func (m *Model) nextLocal() tea.Cmd {
	l := m.local
	cmds := []tea.Cmd{}
	for l.running < 3 && len(l.queue) > 0 {
		p := l.queue[0]
		l.queue = l.queue[1:]
		l.running++
		manager, launcher, editor, parent := l.manager, l.launcher, l.editor, m.ctx
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(parent, 20*time.Minute)
			defer cancel()
			path, err := manager.ExistingPath(ctx, p)
			if err != nil {
				return localResult{p: p, err: err}
			}
			if path != "" {
				if err := ctx.Err(); err != nil {
					return localResult{p: p, err: err}
				}
				if err := launcher.Launch(editor, path); err != nil {
					return localResult{p: p, status: local.Status{Path: path}, err: err}
				}
				return localOpened{p: p, path: path}
			}
			s, err := manager.Prepare(ctx, p)
			if err == nil && ctx.Err() != nil {
				err = ctx.Err()
			}
			if err == nil {
				err = launcher.Launch(editor, s.Path)
			}
			return localResult{p: p, status: s, err: err}
		})
	}
	return tea.Batch(cmds...)
}
func (m *Model) localMessage(msg tea.Msg) (bool, tea.Cmd) {
	if m.local == nil {
		return false, nil
	}
	l := m.local
	switch msg := msg.(type) {
	case localCleanupResult:
		l.cleanupRunning = false
		l.cleanupAt = time.Now()
		if msg.err != nil {
			l.results = append(l.results, "IntelliJ-opprydding: "+msg.err.Error())
		}
		return true, nil
	case localWarmTick:
		return true, tea.Batch(m.warmLocal(), m.cleanupIntelliJ(false), localWarmTimer())
	case localWarmResult:
		l.warming = false
		l.warmed[msg.repository] = time.Now()
		l.warmNotice = "Main klar: " + m.githubName(msg.repository)
		if l.manager.Workspace.Indexer.Command == "" {
			l.warmNotice += " · headless-indeksering ikke konfigurert"
		}
		if msg.err != nil {
			l.warmNotice = fmt.Sprintf("Main %s: %s", m.githubName(msg.repository), msg.err)
			l.results = append(l.results, l.warmNotice)
		}
		return true, m.warmLocal()
	case localStatuses:
		l.statusLoading = false
		for key, s := range msg.values {
			if !l.preparing[key] && msg.generation == l.generation {
				l.statuses[key] = s
			}
		}
		if l.statusAgain {
			l.statusAgain = false
			return true, m.refreshLocal()
		}
		return true, nil
	case localOpened:
		l.running--
		l.updating[msg.p.Key()] = true
		l.statuses[msg.p.Key()] = local.Status{Path: msg.path}
		m.notice = fmt.Sprintf("%s #%d: opened; updating in background", m.githubName(msg.p.Repository), msg.p.Number)
		manager, parent := l.manager, m.ctx
		refresh := func() tea.Msg {
			ctx, cancel := context.WithTimeout(parent, 20*time.Minute)
			defer cancel()
			s, err := manager.Prepare(ctx, msg.p)
			if s.Path == "" {
				s.Path = msg.path
			}
			return localResult{p: msg.p, status: s, err: err, updated: true}
		}
		return true, tea.Batch(m.nextLocal(), refresh)
	case localResult:
		if !msg.updated {
			l.running--
		}
		delete(l.updating, msg.p.Key())
		delete(l.preparing, msg.p.Key())
		s := msg.status
		result := fmt.Sprintf("%s #%d: opened (%s)", m.githubName(msg.p.Repository), msg.p.Number, s.String())
		if msg.err != nil {
			s.Error = msg.err.Error()
			result = fmt.Sprintf("%s #%d: %s", m.githubName(msg.p.Repository), msg.p.Number, msg.err)
			if msg.updated {
				result = fmt.Sprintf("%s #%d: opened; background update failed: %s", m.githubName(msg.p.Repository), msg.p.Number, msg.err)
			}
		}
		l.statuses[msg.p.Key()] = s
		l.results = append(l.results, result)
		m.notice = result
		return true, m.nextLocal()
	}
	return false, nil
}
func (m Model) localLabel(p model.PullRequest) string {
	if m.local == nil {
		return "Not downloaded"
	}
	if m.local.updating[p.Key()] {
		return "Opened · Updating"
	}
	if m.local.preparing[p.Key()] {
		return "Preparing"
	}
	return m.local.statuses[p.Key()].String()
}
