package tui

import (
	"context"
	"reflect"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/app"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/relations"
	"github.com/haakonph/custom-visning-terminal/internal/storage"
	"github.com/haakonph/custom-visning-terminal/internal/tasks"
)

// Only the UI loop owns the queue and applies completions. Workers receive
// immutable snapshots; a single in-flight command preserves write order.
type localIOJob struct {
	run    func() error
	finish func(*Model, error)
}
type localIOState struct {
	quitting bool

	active  *localIOJob
	pending []*localIOJob
}
type localIOResult struct {
	job *localIOJob
	err error
}

func (s *localIOState) enqueue(run func() error, finish func(*Model, error)) {
	s.pending = append(s.pending, &localIOJob{run: run, finish: finish})
}
func (m *Model) enqueueLocal(run func() error, finish func(*Model, error)) {
	if m.localIO == nil {
		m.localIO = &localIOState{}
		m.tasks.io = m.localIO
	}
	m.localIO.enqueue(run, finish)
}
func (s *localIOState) busy() bool { return s != nil && (s.active != nil || len(s.pending) != 0) }
func (m *Model) localBusy() bool {
	if m.localIO.busy() {
		m.sequenceNotice = "Venter på lokal lagring…"
		return true
	}
	return false
}
func (m *Model) nextLocalWrite() tea.Cmd {
	s := m.localIO
	if s == nil || s.active != nil || len(s.pending) == 0 {
		return nil
	}
	job := s.pending[0]
	s.pending[0] = nil
	s.pending = s.pending[1:]
	s.active = job
	return func() tea.Msg { return localIOResult{job: job, err: job.run()} }
}
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Cached lists survive navigation and timer messages, but not data edits.
	if _, ok := msg.(tea.KeyPressMsg); ok && m.localIO != nil && m.localIO.quitting {
		return m, nil
	}
	m.invalidateViews(msg)
	next, cmd := m.observedUpdate(msg)
	result := next.(Model)
	if result.localIO != nil && result.localIO.quitting && result.localIO.active == nil && len(result.localIO.pending) == 0 {
		result.cancel()
		return result, tea.Quit
	}
	if write := result.nextLocalWrite(); write != nil {
		cmd = tea.Batch(cmd, write)
	}
	return result, cmd
}
func cloneTasks(items []model.Task) []model.Task {
	out := append([]model.Task(nil), items...)
	for i := range out {
		out[i].References = append([]model.TaskReference(nil), out[i].References...)
	}
	return out
}

func (m *Model) quit() tea.Cmd {
	if m.localIO != nil && (m.localIO.active != nil || len(m.localIO.pending) != 0) {
		m.localIO.quitting = true
		m.sequenceNotice = "Lagrer før avslutning…"
		return nil
	}
	m.cancel()
	return tea.Quit
}

// Commands capture immutable domain changes. Successful completions install the
// committed result, preserving additions made by other processes.
func (m *Model) queueLocalEdit(state model.EntityState, items []model.Task, taskChanged bool, finish func(*Model, error)) {
	changes := app.LocalChanges{}
	var err error
	changes.Relations, err = relations.Diff(m.entities.data, state)
	if err != nil {
		finish(m, err)
		return
	}
	if taskChanged {
		if err = tasks.EnsureIDs(items); err != nil {
			finish(m, err)
			return
		}
		changes.Tasks, err = tasks.Changes(m.tasks.saved, items)
		if err != nil {
			finish(m, err)
			return
		}
	}
	m.commitLocal(changes, taskChanged, finish)
}
func (m *Model) commitLocal(changes app.LocalChanges, replaceDraft bool, finish func(*Model, error)) {
	store := storage.LocalStore{Path: m.tasks.path}
	var result app.LocalResult
	m.enqueueLocal(func() (err error) {
		result, err = app.Commit(context.Background(), store, changes)
		return err
	}, func(m *Model, err error) {
		if err == nil {
			if replaceDraft || reflect.DeepEqual(m.tasks.items, m.tasks.saved) {
				m.tasks.items = result.Tasks
				m.tasks.saved = tasks.Clone(result.Tasks)
			}
			m.entities.data = result.Relations
			if m.snoozes != nil {
				m.snoozes.entries = result.Snoozes
				m.snoozes.visibilityVersion++
				m.snoozes.indexContexts()
			}
		}
		finish(m, err)
	})
}
