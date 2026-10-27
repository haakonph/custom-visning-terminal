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
		return m.snapshot.Datasets[view]
	}
	personal := m.snapshot.Datasets[model.Reviews]
	team := m.snapshot.Datasets[model.TeamReviews]
	data := model.Dataset{SyncedAt: personal.SyncedAt}
	if team.SyncedAt.Before(data.SyncedAt) {
		data.SyncedAt = team.SyncedAt
	}
	seen := map[string]bool{}
	for _, source := range []model.View{model.Reviews, model.TeamReviews} {
		for _, p := range m.snapshot.Datasets[source].PRs {
			key := strings.ToLower(p.URL)
			if seen[key] || m.reviewRequestedAt(p.URL).IsZero() {
				continue
			}
			if source == model.TeamReviews && strings.EqualFold(p.Author, m.snapshot.User) {
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
		return m.snapshot.Datasets[view].SyncedAt
	}
	personal, team := m.snapshot.Datasets[model.Reviews].SyncedAt, m.snapshot.Datasets[model.TeamReviews].SyncedAt
	if team.Before(personal) {
		return team
	}
	return personal
}
