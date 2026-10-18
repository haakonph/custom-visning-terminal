package tui

import (
	"fmt"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"sort"
)

// selectedPRs resolves the persistent selection against the latest snapshot.
// Without a selection, commands act on the highlighted row.
func (m Model) selectedPRs() []model.PullRequest {
	rows := make([]model.PullRequest, 0, len(m.chosen))
	for _, p := range m.chosen {
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
		if m.selected >= 0 && m.selected < len(visible) {
			rows = append(rows, visible[m.selected])
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Repository == rows[j].Repository {
			return rows[i].Number < rows[j].Number
		}
		return rows[i].Repository < rows[j].Repository
	})
	return rows
}

func (m Model) taskReferences() []model.TaskReference {
	if m.jiraActive {
		if ref, ok := m.taskReference(); ok {
			return []model.TaskReference{ref}
		}
		return nil
	}
	role := ""
	if m.active == model.Reviews || m.active == model.TeamReviews {
		role = "review"
	}
	var refs []model.TaskReference
	for _, p := range m.selectedPRs() {
		refs = append(refs, model.TaskReference{Provider: "github", URL: p.URL, Key: fmt.Sprintf("%s#%d", p.Repository, p.Number), Title: p.Title, Role: role})
	}
	return refs
}
