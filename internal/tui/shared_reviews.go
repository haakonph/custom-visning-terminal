package tui

import (
	"strings"
	"time"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

// Derive shared requests from live review datasets and the existing message index.
// Keeping this out of Snapshot preserves cache compatibility and avoids extra API calls.
func (m Model) githubDataset(view model.View) model.Dataset {
	if view != model.SharedReviews {
		return m.prs.Snapshot().Datasets[view]
	}
	personal := m.prs.Snapshot().Datasets[model.Reviews]
	team := m.prs.Snapshot().Datasets[model.TeamReviews]
	data := model.Dataset{SyncedAt: personal.SyncedAt}
	if team.SyncedAt.Before(data.SyncedAt) {
		data.SyncedAt = team.SyncedAt
	}
	seen := map[string]bool{}
	for _, source := range []model.View{model.Reviews, model.TeamReviews} {
		for _, p := range m.prs.Snapshot().Datasets[source].PRs {
			key := strings.ToLower(p.URL)
			if seen[key] || m.reviewRequestedAt(p.URL).IsZero() {
				continue
			}
			if source == model.TeamReviews && strings.EqualFold(p.Author, m.prs.Snapshot().User) {
				continue
			}
			seen[key] = true
			data.PRs = append(data.PRs, p)
		}
	}
	model.Sort(data.PRs)
	return data
}

func (m Model) githubLoading(view model.View) bool {
	if view == model.SharedReviews {
		return m.loading[model.Reviews] || m.loading[model.TeamReviews]
	}
	return m.loading[view]
}

func (m Model) githubWarning(view model.View) string {
	if view == model.SharedReviews {
		var warnings []string
		for _, source := range []model.View{model.Reviews, model.TeamReviews} {
			if warning := m.warnings[source]; warning != "" {
				warnings = append(warnings, source.String()+": "+warning)
			}
		}
		return strings.Join(warnings, "; ")
	}
	return m.warnings[view]
}

func (m Model) githubSyncedAt(view model.View) time.Time {
	if view != model.SharedReviews {
		return m.prs.Snapshot().Datasets[view].SyncedAt
	}
	personal, team := m.prs.Snapshot().Datasets[model.Reviews].SyncedAt, m.prs.Snapshot().Datasets[model.TeamReviews].SyncedAt
	if team.Before(personal) {
		return team
	}
	return personal
}
