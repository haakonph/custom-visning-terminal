package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/pullrequests"
)

type copilotReviewRequested struct{ result pullrequests.ReviewResult }

func (m Model) startCopilotReview(refs []model.TaskReference) (tea.Model, tea.Cmd) {
	if !allProvider(refs, "github") {
		return m.unavailable()
	}
	run, err := m.prs.RequestReview(m.ctx, prIDs(refs))
	if err != nil {
		m.sequenceNotice = err.Error()
		return m, nil
	}
	m.sequenceNotice = "Ber om ny Copilot-review…"
	return m, func() tea.Msg { return copilotReviewRequested{result: run()} }
}
