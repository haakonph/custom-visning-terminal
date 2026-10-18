package tui

import (
	"context"
	"fmt"
	"sort"
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
	queue                      []model.PullRequest
	running                    int
	editor                     string
	choosing                   bool
	results                    []string
}
type localResult struct {
	p      model.PullRequest
	status local.Status
	err    error
}
type localStatuses struct {
	values     map[string]local.Status
	generation uint64
}

func (m Model) WithLocal(manager *local.WorktreeManager, editors local.Editors) Model {
	m.local = &localState{manager: manager, launcher: local.EditorLauncher{Editors: editors}, chosen: map[string]model.PullRequest{}, statuses: map[string]local.Status{}, preparing: map[string]bool{}}
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
	return func() tea.Msg {
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
}
func (m *Model) openLocal(editor string) tea.Cmd {
	if m.local == nil {
		m.notice = "Local worktrees unavailable; check startup configuration"
		return nil
	}
	l := m.local
	if l.running > 0 {
		m.notice = "An open operation is already running"
		return nil
	}
	// Validate the editor before any clone or fetch.
	if _, err := l.launcher.Command(editor, "."); err != nil {
		m.notice = err.Error()
		return nil
	}
	rows := []model.PullRequest{}
	for _, p := range l.chosen {
		// Use the latest dashboard head even if selection predates a refresh.
		for _, d := range m.snapshot.Datasets {
			for _, fresh := range d.PRs {
				if fresh.Key() == p.Key() && !fresh.UpdatedAt.Before(p.UpdatedAt) {
					p = fresh
				}
			}
		}
		rows = append(rows, p)
	}
	if len(rows) == 0 {
		visible := m.filtered()
		if len(visible) == 0 {
			return nil
		}
		rows = append(rows, visible[m.selected])
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Repository == rows[j].Repository {
			return rows[i].Number < rows[j].Number
		}
		return rows[i].Repository < rows[j].Repository
	})
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

// At most three Git jobs are scheduled. The repository manager serializes all
// operations for one repository, including worktree metadata changes.
func (m *Model) nextLocal() tea.Cmd {
	l := m.local
	cmds := []tea.Cmd{}
	for l.running < 3 && len(l.queue) > 0 {
		p := l.queue[0]
		l.queue = l.queue[1:]
		l.running++
		manager, launcher, editor, parent := l.manager, l.launcher, l.editor, m.ctx
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
			defer cancel()
			s, err := manager.Prepare(ctx, p)
			if err == nil && ctx.Err() != nil {
				err = ctx.Err()
			}
			if err == nil {
				err = launcher.Launch(editor, s.Path)
			}
			return localResult{p, s, err}
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
	case localResult:
		l.running--
		delete(l.preparing, msg.p.Key())
		s := msg.status
		result := fmt.Sprintf("%s #%d: opened (%s)", msg.p.Repository, msg.p.Number, s.String())
		if msg.err != nil {
			s.Error = msg.err.Error()
			result = fmt.Sprintf("%s #%d: %s", msg.p.Repository, msg.p.Number, msg.err)
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
	if m.local.preparing[p.Key()] {
		return "Preparing"
	}
	return m.local.statuses[p.Key()].String()
}
