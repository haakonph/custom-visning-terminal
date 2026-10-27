package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

type viewCache struct {
	groups *prGroupLayout

	sources       [model.ViewCount][]model.PullRequest
	archived      []model.TaskReference
	snoozeVersion uint64
	user          string

	githubValid          bool
	githubView           model.View
	githubQuery, related string
	githubRows           []model.PullRequest
}

func navigationMessage(msg tea.Msg) bool {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "j", "k", "up", "down", "h", "l", "left", "right", "tab", "shift+tab":
			return true
		}
	}
	switch msg.(type) {
	case refreshTick, copilotTick, mmTick:
		return true
	}
	return false
}
func (m *Model) invalidateViews(msg tea.Msg) {
	if navigationMessage(msg) {
		return
	}
	// The one-second snooze timer only changes visibility at a deadline.
	if _, ok := msg.(snoozeTick); ok && m.snoozes != nil {
		now := m.snoozes.now()
		expired := false
		for _, e := range m.snoozes.entries {
			if e.Until.After(m.snoozes.at) && !e.Until.After(now) {
				expired = true
				break
			}
		}
		if !expired {
			return
		}
	}
	if m.snoozes != nil {
		m.snoozes.contextDirty = true
	}
	if m.views != nil {
		m.views.githubValid = false
		m.views.groups = nil
	}
	if m.detail != nil {
		m.detail.lines = nil
	}
	if m.copilot != nil {
		m.copilot.formattedDetail = nil
	}
	if m.mmReader != nil {
		m.mmReader.filteredValid = false
	}
}
func (m Model) filtered() []model.PullRequest {
	c := m.views
	q := strings.ToLower(strings.TrimSpace(m.input.Value()))
	same := c != nil && c.user == m.prs.Snapshot().User && len(c.archived) == len(m.entities.data.Archived) && (len(c.archived) == 0 || &c.archived[0] == &m.entities.data.Archived[0])
	if same {
		for v := range m.prs.Snapshot().Datasets {
			if !sameRows(c.sources[v], m.prs.Snapshot().Datasets[v].PRs) {
				same = false
				break
			}
		}
	}
	if same && m.snoozes != nil {
		same = c.snoozeVersion == m.snoozes.visibilityVersion
	}
	if same && c.githubValid && c.githubView == m.active && c.githubQuery == q && c.related == m.relatedKey {
		return c.githubRows
	}
	rows := m.filterGitHubRows()
	if c != nil {
		c.groups = nil
		for v := range m.prs.Snapshot().Datasets {
			c.sources[v] = m.prs.Snapshot().Datasets[v].PRs
		}
		c.archived, c.user = m.entities.data.Archived, m.prs.Snapshot().User
		if m.snoozes != nil {
			c.snoozeVersion = m.snoozes.visibilityVersion
		}
		c.githubValid, c.githubView, c.githubQuery, c.related, c.githubRows = true, m.active, q, m.relatedKey, rows
	}
	return rows
}
