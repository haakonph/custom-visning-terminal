package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

type localRemoveResult struct {
	rows   []model.PullRequest
	errors []error
}

func (m Model) beginRemoveLocal(refs []model.TaskReference) (tea.Model, tea.Cmd) {
	if m.local == nil || !allProvider(refs, "github") {
		return m.unavailable()
	}
	if m.local.running > 0 || len(m.local.preparing) > 0 || m.local.removing {
		m.sequenceNotice = "Vent til worktree-operasjonen er ferdig."
		return m, nil
	}
	var rows []model.PullRequest
	seen := map[string]bool{}
	for _, ref := range refs {
		found := false
		for _, dataset := range m.snapshot.Datasets {
			for _, p := range dataset.PRs {
				if p.URL == ref.URL {
					found = true
					if !seen[p.Key()] {
						rows = append(rows, p)
						seen[p.Key()] = true
					}
				}
			}
		}
		if !found {
			return m.unavailable()
		}
	}
	if len(rows) == 0 {
		return m.unavailable()
	}
	m.local.removePending = rows
	return m, nil
}

func (m Model) removeLocalKey(k string) (tea.Model, tea.Cmd, bool) {
	if k == "esc" || k == "n" {
		m.local.removePending = nil
		return m, nil, true
	}
	if k != "y" {
		return m, nil, true
	}
	rows := m.local.removePending
	m.local.removePending = nil
	m.local.removing = true
	m.local.generation++
	for _, p := range rows {
		m.local.preparing[p.Key()] = true
	}
	m.notice = "Sletter lokale worktrees…"
	manager, parent := m.local.manager, m.ctx
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
		defer cancel()
		result := localRemoveResult{rows: rows}
		for _, p := range rows {
			result.errors = append(result.errors, manager.Remove(ctx, p))
		}
		return result
	}, true
}

func (m Model) removeLocalView() tea.View {
	lines := []string{"Slette lokale worktrees?", ""}
	shown := min(len(m.local.removePending), max(1, m.height-10))
	for _, p := range m.local.removePending[:shown] {
		lines = append(lines, fmt.Sprintf("  %s #%d · %s", clean(p.Repository), p.Number, clean(p.Title)))
	}
	if shown < len(m.local.removePending) {
		lines = append(lines, fmt.Sprintf("  … og %d flere valgte PR-er", len(m.local.removePending)-shown))
	}
	lines = append(lines, "", "Worktree-mappene, inkludert ignorerte filer, fjernes.", "Lokale endringer og usporede filer stopper sletting. Grener og commits beholdes.", m.keyText("Bruk `ee` etter sletting for å opprette et nytt worktree."), "", "y slett · n/Esc avbryt")
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	return view
}
