package tui

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

type autoMergeMsg struct {
	updated []model.PullRequest
	err     error
}

func (m Model) startAutoMerge(refs []model.TaskReference, enabled bool) (tea.Model, tea.Cmd) {
	client, ok := m.source.(model.AutoMergeSource)
	if !ok || !allProvider(refs, "github") {
		return m.unavailable()
	}
	if m.autoMergeBusy {
		m.sequenceNotice = "Automerge oppdateres allerede…"
		return m, nil
	}
	var prs []model.PullRequest
	seen := map[string]bool{}
	for _, ref := range refs {
		found := false
		for _, dataset := range m.snapshot.Datasets {
			for _, p := range dataset.PRs {
				if p.URL == ref.URL {
					found = true
					if !seen[p.URL] {
						prs = append(prs, p)
						seen[p.URL] = true
					}
				}
			}
		}
		if !found {
			return m.unavailable()
		}
	}
	m.autoMergeBusy = true
	m.autoMergeVersion++
	m.sequenceNotice = "Oppdaterer automerge…"
	parent := m.ctx
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
		defer cancel()
		result := autoMergeMsg{}
		for _, p := range prs {
			updated, err := client.SetAutoMerge(ctx, p, enabled)
			if err != nil {
				result.err = fmt.Errorf("%s #%d: %w (%d PR-er oppdatert; resten stoppet)", p.Repository, p.Number, err, len(result.updated))
				break
			}
			result.updated = append(result.updated, updated)
		}
		return result
	}
}

func (m Model) finishAutoMerge(result autoMergeMsg) (tea.Model, tea.Cmd) {
	m.autoMergeBusy = false
	m.autoMergeVersion++
	for _, p := range result.updated {
		patch := func(row *model.PullRequest) {
			row.AutoMerge, row.MergeQueue, row.QueuePosition = p.AutoMerge, p.MergeQueue, p.QueuePosition
		}
		for v := range m.snapshot.Datasets {
			// Copy before editing: a cache write may still hold the old snapshot.
			rows := append([]model.PullRequest(nil), m.snapshot.Datasets[v].PRs...)
			for i := range rows {
				if rows[i].URL == p.URL {
					patch(&rows[i])
				}
			}
			m.snapshot.Datasets[v].PRs = rows
		}
		if row, ok := m.chosen[p.Key()]; ok {
			patch(&row)
			m.chosen[p.Key()] = row
		}
		if m.detail != nil && m.detail.pr.URL == p.URL {
			patch(&m.detail.pr)
		}
	}
	m.sequenceNotice = fmt.Sprintf("Automerge oppdatert for %d PR-er", len(result.updated))
	if result.err != nil {
		m.sequenceNotice = result.err.Error()
	}
	return m, m.persist()
}
