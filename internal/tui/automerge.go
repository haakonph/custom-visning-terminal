package tui

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/pullrequests"
)

type autoMergeMsg struct{ result pullrequests.AutoMergeResult }

func prIDs(refs []model.TaskReference) []string {
	ids := make([]string, 0, len(refs))
	for _, r := range refs {
		ids = append(ids, r.URL)
	}
	return ids
}
func (m Model) startAutoMerge(refs []model.TaskReference, enabled bool) (tea.Model, tea.Cmd) {
	if !allProvider(refs, "github") {
		return m.unavailable()
	}
	run, err := m.prs.SetAutoMerge(m.ctx, prIDs(refs), enabled)
	if err != nil {
		m.sequenceNotice = err.Error()
		return m, nil
	}
	m.sequenceNotice = "Oppdaterer automerge…"
	return m, func() tea.Msg { return autoMergeMsg{result: run()} }
}
func (m Model) finishAutoMerge(msg autoMergeMsg) (tea.Model, tea.Cmd) {
	if !m.prs.ApplyAutoMerge(msg.result) {
		return m, nil
	}
	m.sequenceNotice = fmt.Sprintf("Automerge oppdatert for %d PR-er", len(msg.result.Updated))
	if msg.result.Err != nil {
		m.sequenceNotice = msg.result.Err.Error()
	}
	return m, m.persist()
}
