package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

const dependabotGroup = "Dependabot"

// Review requests from Dependabot share one group. Other PRs appear once,
// under their first Jira key, or the issue being viewed in related views.
func (m Model) prGroupKey(p model.PullRequest) string {
	if m.active.IsReview() &&
		(strings.EqualFold(p.Author, "dependabot") || strings.EqualFold(p.Author, "dependabot[bot]")) {
		return dependabotGroup
	}
	if m.relatedKey != "" {
		return m.relatedKey
	}
	if keys := p.JiraKeys(m.jiraPattern); len(keys) > 0 {
		return keys[0]
	}
	return ""
}

// Sort PRs by repository within groups and order groups by their first PR.
// Keep unlinked PRs together, followed by Dependabot review requests, without
// modifying the cached dataset.
func (m Model) groupPRs(rows []model.PullRequest) []model.PullRequest {
	groups := map[string][]model.PullRequest{}
	var keys []string
	for _, p := range rows {
		key := m.prGroupKey(p)
		if _, exists := groups[key]; !exists && key != "" && key != dependabotGroup {
			keys = append(keys, key)
		}
		groups[key] = append(groups[key], p)
	}
	keys = append(keys, "", dependabotGroup)
	result := make([]model.PullRequest, 0, len(rows))
	for _, key := range keys {
		sort.SliceStable(groups[key], func(i, j int) bool {
			return groups[key][i].Repository < groups[key][j].Repository
		})
		result = append(result, groups[key]...)
	}
	return result
}

func (m Model) prGroupHeading(key string, count int) string {
	label := key
	if key == "" {
		label = "Uten Jira-sak"
	} else if key != dependabotGroup && m.jira != nil {
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
	layout := m.prLayout(rows)
	counts, stacks, entries := layout.counts, layout.stacks, layout.entries
	selectedLine := 0
	if m.selected >= 0 && m.selected < len(layout.rowLines) {
		selectedLine = layout.rowLines[m.selected]
	}
	start := max(0, selectedLine-capacity+1)
	var lines []string
	if start > 0 && entries[start].row >= 0 && capacity > 1 {
		key := entries[start].heading
		start++
		if entries[start].row >= 0 {
			lines = append(lines, m.prGroupHeading(key, counts[key]))
		}
	}
	for _, e := range entries[start:min(len(entries), start+capacity-len(lines))] {
		if e.row < 0 {
			lines = append(lines, m.prGroupHeading(e.heading, counts[e.heading]))
			continue
		}
		rowModel := m
		rowModel.width = max(1, m.width-3)
		p := rows[e.row]
		lines = append(lines, "   "+rowModel.stackRow(p, e.row == m.selected, stacks[p.Key()]))
	}
	return lines
}

// Add stack position to the title, preserving the original PR identity.
func (m Model) stackRow(p model.PullRequest, selected bool, stack model.StackPosition) string {
	if stack.Total > 1 {
		p.Title = fmt.Sprintf("[stack %d/%d] %s", stack.Position, stack.Total, p.Title)
	}
	return m.row(p, false, selected)
}

// Group structure and stack positions depend on data, never on the cursor.
// Cache these alongside the filtered list, then format only viewport rows.
type prGroupEntry struct {
	row     int
	heading string
}
type prGroupLayout struct {
	rows     []model.PullRequest
	counts   map[string]int
	stacks   map[string]model.StackPosition
	entries  []prGroupEntry
	rowLines []int
}

func (m Model) prLayout(rows []model.PullRequest) *prGroupLayout {
	if m.views != nil && m.views.groups != nil && sameRows(m.views.groups.rows, rows) {
		return m.views.groups
	}
	counts := map[string]int{}
	for _, p := range rows {
		counts[m.prGroupKey(p)]++
	}
	var known []model.PullRequest
	for _, dataset := range m.snapshot.Datasets {
		known = append(known, dataset.PRs...)
	}
	known = append(known, rows...)
	layout := &prGroupLayout{rows: rows, counts: counts, stacks: model.PRStacks(known), rowLines: make([]int, len(rows))}
	for i, p := range rows {
		key := m.prGroupKey(p)
		if i == 0 || key != m.prGroupKey(rows[i-1]) {
			layout.entries = append(layout.entries, prGroupEntry{row: -1, heading: key})
		}
		layout.rowLines[i] = len(layout.entries)
		layout.entries = append(layout.entries, prGroupEntry{row: i, heading: key})
	}
	if m.views != nil {
		m.views.groups = layout
	}
	return layout
}
