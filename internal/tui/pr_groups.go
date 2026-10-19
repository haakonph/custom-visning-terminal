package tui

import (
	"fmt"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

// A PR appears once, under its first Jira key. Related views use the issue
// being viewed, even when it is not the first key mentioned in the PR.
func (m Model) prGroupKey(p model.PullRequest) string {
	if m.relatedKey != "" {
		return m.relatedKey
	}
	if keys := p.JiraKeys(m.jiraPattern); len(keys) > 0 {
		return keys[0]
	}
	return ""
}

// Preserve the source order within groups and order groups by their first PR.
// Keep unlinked PRs together at the end without modifying the cached dataset.
func (m Model) groupPRs(rows []model.PullRequest) []model.PullRequest {
	groups := map[string][]model.PullRequest{}
	var keys []string
	for _, p := range rows {
		key := m.prGroupKey(p)
		if _, exists := groups[key]; !exists && key != "" {
			keys = append(keys, key)
		}
		groups[key] = append(groups[key], p)
	}
	keys = append(keys, "")
	result := make([]model.PullRequest, 0, len(rows))
	for _, key := range keys {
		result = append(result, groups[key]...)
	}
	return result
}

func (m Model) prGroupHeading(key string, count int) string {
	label := key
	if key == "" {
		label = "Uten Jira-sak"
	} else if m.jira != nil {
		for _, issue := range m.jira.rows {
			if issue.Key == key {
				label += " · " + issue.Title
				break
			}
		}
	}
	return accent.Render(clean(fmt.Sprintf(" ▼ %s · %d PR", label, count)))
}

// Headers are presentation only: selection, ranges and actions all use the
// same flattened PR order. Repeat the group heading when scrolling inside it.
func (m Model) groupedPRLines(rows []model.PullRequest, capacity int) []string {
	if len(rows) == 0 {
		return nil
	}
	capacity = max(1, capacity)
	counts := map[string]int{}
	for _, p := range rows {
		counts[m.prGroupKey(p)]++
	}
	var lines, headings []string
	selectedLine := 0
	for i, p := range rows {
		key := m.prGroupKey(p)
		heading := m.prGroupHeading(key, counts[key])
		if i == 0 || key != m.prGroupKey(rows[i-1]) {
			lines = append(lines, heading)
			headings = append(headings, "")
		}
		if i == m.selected {
			selectedLine = len(lines)
		}
		rowModel := m
		rowModel.width = max(1, m.width-3)
		lines = append(lines, "   "+rowModel.row(p, false, i == m.selected))
		headings = append(headings, heading)
	}
	start := max(0, selectedLine-capacity+1)
	if start > 0 && headings[start] != "" && capacity > 1 {
		// Reserve one line for context without pushing the selected PR out.
		start++
		if headings[start] == "" {
			return lines[start:min(len(lines), start+capacity)]
		}
		return append([]string{headings[start-1]}, lines[start:min(len(lines), start+capacity-1)]...)
	}
	return lines[start:min(len(lines), start+capacity)]
}
