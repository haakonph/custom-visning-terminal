package tui

import (
	"fmt"
	"strings"

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
	if p.AutoMerge == "" {
		return string(model.Unknown)
	}
	return string(p.AutoMerge)
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
