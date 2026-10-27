package tui

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestWorktreeResultsAcrossCopilotAndTasks(t *testing.T) {
	t.Setenv("CUSTOM_VISNING_TASKS", filepath.Join(t.TempDir(), "tasks.json"))
	m := localModel(t).WithCopilot(testCopilot(t), false)
	p := m.filtered()[0]
	m, _ = update(m, key(' '))
	m.local.running = 1
	m.local.preparing[p.Key()] = true
	m, _ = sequence(m, "va")
	m, _ = update(m, localResult{p: p, err: errors.New("editor unavailable")})
	if !(m.page == pageCopilot) || m.local.running != 0 || m.local.statuses[p.Key()].Error == "" {
		t.Fatal("worktree result lost while Copilot was active")
	}
	m, _ = sequence(m, "vg")
	m, _ = sequence(m, "an")
	if !(m.page == pageTasks) || len(m.tasks.items) != 1 {
		t.Fatal("task creation failed after returning from Copilot")
	}
	m, _ = sequence(m, "ae")
	title := m.tasks.input.Value()
	m.local.running = 1
	m, _ = update(m, localResult{p: p})
	m, _ = update(m, copilotResult{})
	if !(m.page == pageTasks) || !m.tasks.editing || m.tasks.input.Value() != title || m.local.running != 0 {
		t.Fatal("background results interrupted task editing")
	}
	m, _ = update(m, key('z'))
	if m.tasks.input.Value() == title {
		t.Fatal("task editor no longer receives input")
	}
	m.tasks.editing = false
	m, _ = sequence(m, "vg")
	if (m.page == pageTasks) || (m.page == pageCopilot) || len(m.local.chosen) != 1 {
		t.Fatal("provider navigation lost PR selection")
	}
	if m.tasks.items[0].References[0].Provider != "github" || m.active != model.Open {
		t.Fatal("task reference changed across provider navigation")
	}
}
