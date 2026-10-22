package tui

import (
	"fmt"
	"strings"
	"time"

	appconfig "github.com/haakonph/custom-visning-terminal/internal/config"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

var defaultGitHubPrefixes = []string{"skatteetaten-ghe"}

func (m *Model) loadGitHubDisplay() {
	cfg := struct {
		HiddenPrefixes []string `json:"hidden_prefixes"`
	}{HiddenPrefixes: append([]string{}, defaultGitHubPrefixes...)}
	if err := appconfig.Section(appconfig.Path(), "github", &cfg); err != nil {
		m.notice = "Kunne ikke lese GitHub-visning: " + err.Error()
		return
	}
	m.githubHiddenPrefixes = cfg.HiddenPrefixes
}

// Names are shortened only when rendering. URLs and stored identities stay intact.
func (m Model) githubName(name string) string {
	name = m.githubOrganizationName(name)
	if owner, repository, ok := strings.Cut(name, "/"); ok {
		return owner + "/" + strings.TrimPrefix(repository, "glsk-")
	}
	return strings.TrimPrefix(name, "glsk-")
}

func (m Model) githubOrganizationName(name string) string {
	prefixes := m.githubHiddenPrefixes
	if prefixes == nil {
		prefixes = defaultGitHubPrefixes
	}
	for _, prefix := range prefixes {
		prefix = strings.TrimSuffix(strings.TrimSpace(prefix), "/")
		if prefix == "" {
			continue
		}
		prefix += "/"
		if len(name) >= len(prefix) && strings.EqualFold(name[:len(prefix)], prefix) {
			return name[len(prefix):]
		}
	}
	return name
}

func (m Model) githubTeams(teams []string) string {
	names := make([]string, len(teams))
	for i, team := range teams {
		names[i] = m.githubOrganizationName(team)
	}
	return strings.Join(names, ", ")
}

func (m Model) displayReference(r model.TaskReference) model.TaskReference {
	if r.Provider != "github" {
		return r
	}
	keys := (model.PullRequest{Title: r.Title}).JiraKeys(m.jiraPattern)
	r.Key = m.githubName(r.Key)
	r.Title = (model.PullRequest{Title: r.Title}).DisplayTitle(m.jiraPattern)
	if len(keys) > 0 {
		r.Title += " · Jira: " + strings.Join(keys, ", ")
	}
	return r
}

func autoMergeLabel(p model.PullRequest) string {
	if p.AutoMerge == "ON" {
		return "ON"
	}
	return ""
}

// prInfoLabel prioritizes merge blockers in the compact table cell.
// The full queue state remains available in the selected PR and detail views.
func prInfoLabel(p model.PullRequest) string {
	status := queueLabel(p)
	if p.Mergeability == model.Conflicting {
		status = string(model.Conflicting)
	}
	parts := []string{}
	if status != "" && status != string(model.Unknown) {
		parts = append(parts, status)
	}
	if p.AutoMerge == "ON" {
		parts = append(parts, "auto")
	}
	if p.OpenComments != nil && *p.OpenComments > 0 {
		parts = append(parts, fmt.Sprintf("%d åpne", *p.OpenComments))
	}
	if len(parts) == 0 {
		return status
	}
	return strings.Join(parts, " · ")
}

func queueLabel(p model.PullRequest) string {
	state := string(p.MergeQueue)
	if state == "NONE" {
		return ""
	}
	if state == "" {
		state = string(model.Unknown)
	}
	if p.QueuePosition > 0 {
		return fmt.Sprintf("#%d %s", p.QueuePosition, state)
	}
	return state
}

func reviewSummaryLabel(p model.PullRequest) string {
	state := p.Review
	if state == "" {
		state = model.Unknown
	}
	label := "Review: " + string(state)
	if p.CopilotReview != nil || copilotReviewPending(p) {
		label += " · Copilot: " + copilotReviewLabel(p)
	}
	s := p.ReviewSummary
	if s == nil {
		return label
	}
	if s.Fallback {
		label += " (vurderinger)"
	} else {
		label += " (GitHub)"
	}
	label += fmt.Sprintf(" · %d godkjent · %d endringer · %d venter", s.Approved, s.Changes, s.Pending)
	if s.PendingTeams > 0 {
		label += fmt.Sprintf(" · %d team venter", s.PendingTeams)
	}
	if !s.Complete {
		label += " · delvis opptelling"
	}
	return label
}

func copilotReviewPending(p model.PullRequest) bool {
	for _, reviewer := range p.RequestedReviewers {
		login := strings.TrimSuffix(strings.ToLower(reviewer), "[bot]")
		if login == "copilot" || login == "copilot-pull-request-reviewer" {
			return true
		}
	}
	return false
}

func copilotReviewLabel(p model.PullRequest) string {
	if copilotReviewPending(p) {
		return "⏳"
	}
	r := p.CopilotReview
	if r == nil {
		return "—"
	}
	icon := "💬"
	switch r.Recommendation {
	case "Approval":
		icon = "🟢"
	case "Changes":
		icon = "🔴"
	case "Review":
		icon = "🟡"
	case "Dismissed":
		icon = "⚪"
	}
	if r.Comments > 0 {
		return fmt.Sprintf("%s %d", icon, r.Comments)
	}
	return icon
}

func (m Model) reviewLabel(p model.PullRequest) string {
	if m.active == model.Reviews || m.active == model.TeamReviews {
		if own := m.ownReview(p); own != "" {
			return own
		}
		if at := m.reviewRequestedAt(p.URL); !at.IsZero() {
			return age(at)
		}
		return ""
	}

	if p.Review == model.Pending && m.mattermost != nil {
		if at := m.mattermost.sent[p.URL]; !at.IsZero() {
			return age(at)
		}
	}
	return string(p.Review)
}

// Current opinions identify the viewer independently of the branch protection decision.
func (m Model) ownReview(p model.PullRequest) string {
	if p.ReviewSummary == nil || m.snapshot.User == "" {
		return ""
	}
	for login, state := range p.ReviewSummary.Reviewers {
		if strings.EqualFold(login, m.snapshot.User) {
			switch state {
			case "APPROVED":
				return string(model.Approved)
			case "CHANGES_REQUESTED":
				return string(model.ChangesRequested)
			}
		}
	}
	return ""
}

func (m Model) reviewColor(p model.PullRequest) string {
	switch m.ownReview(p) {
	case string(model.Approved):
		return "108"
	case string(model.ChangesRequested):
		return "174"
	}
	if p.ReviewSummary != nil && p.ReviewSummary.Approved+p.ReviewSummary.Changes > 0 {
		return "108"
	}
	return "179"
}

func (m Model) reviewRequestedAt(url string) time.Time {
	key := strings.ToLower(url)
	var at time.Time
	if m.mattermost != nil {
		at = m.mattermost.sent[key]
	}
	if m.mmReader != nil {
		if channelTime := m.mmReader.reviewRequests[key]; channelTime.After(at) {
			at = channelTime
		}
	}
	return at
}
