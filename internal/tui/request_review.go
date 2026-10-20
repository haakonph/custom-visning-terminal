package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"fmt"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"time"
)

type copilotReviewRequested struct {
	count int
	err   error
}

func (m Model) startCopilotReview(refs []model.TaskReference) (tea.Model, tea.Cmd) {
	client, ok := m.source.(model.CopilotReviewSource)
	if !ok || !allProvider(refs, "github") {
		return m.unavailable()
	}
	if m.reviewRequestBusy {
		m.sequenceNotice = "Copilot-review forespørres allerede…"
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
	m.reviewRequestBusy = true
	m.sequenceNotice = "Ber om ny Copilot-review…"
	parent := m.ctx
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
		defer cancel()
		result := copilotReviewRequested{}
		for _, p := range prs {
			if err := client.RequestCopilotReview(ctx, p); err != nil {
				result.err = fmt.Errorf("%s #%d: %w (%d forespørsler sendt; resten stoppet)", p.Repository, p.Number, err, result.count)
				break
			}
			result.count++
		}
		return result
	}
}
