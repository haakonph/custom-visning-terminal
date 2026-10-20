package model

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// StackPosition numbers the known PRs in a connected stack. Cyclic references
// use a stable order; they cannot establish a merge order.
type StackPosition struct{ Position, Total int }

var prLink = regexp.MustCompile(`https?://[^\s<>()\[\]]+/pull/[0-9]+\b`)
var prNumber = regexp.MustCompile(`(?:^|[^\w/#])#([0-9]+)\b`)

// PRStacks uses branch dependencies and body links across all known views.
// Missing PRs are not invented, and unrelated PRs receive no stack label.
func PRStacks(rows []PullRequest) map[string]StackPosition {
	byKey := map[string]PullRequest{}
	for _, p := range rows {
		if p.Key() == "" {
			continue
		}
		old, ok := byKey[p.Key()]
		if !ok || !p.UpdatedAt.Before(old.UpdatedAt) {
			byKey[p.Key()] = p
		}
	}
	keys := make([]string, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parents, neighbors := map[string]map[string]bool{}, map[string]map[string]bool{}
	for _, k := range keys {
		parents[k] = map[string]bool{}
		neighbors[k] = map[string]bool{}
	}
	link := func(child, parent string) {
		if child == parent {
			return
		}
		parents[child][parent] = true
		neighbors[child][parent] = true
		neighbors[parent][child] = true
	}
	for _, k := range keys {
		p := byKey[k]
		urls := map[string]bool{}
		for _, u := range prLink.FindAllString(p.Body, -1) {
			urls[u] = true
		}
		numbers := map[int]bool{}
		for _, match := range prNumber.FindAllStringSubmatch(prLink.ReplaceAllString(p.Body, ""), -1) {
			n, _ := strconv.Atoi(match[1])
			numbers[n] = true
		}
		for _, other := range keys {
			q := byKey[other]
			// A repository name alone does not distinguish GitHub hosts.
			sameRepo := strings.EqualFold(p.Repository, q.Repository) &&
				strings.EqualFold(strings.Split(p.URL, "/pull/")[0], strings.Split(q.URL, "/pull/")[0])
			if urls[q.URL] || sameRepo && (numbers[q.Number] || p.BaseRef != "" && p.BaseRef == q.HeadRef) {
				link(k, other)
			}
		}
	}
	result := map[string]StackPosition{}
	seen := map[string]bool{}
	for _, root := range keys {
		if seen[root] || len(neighbors[root]) == 0 {
			continue
		}
		component := map[string]bool{}
		var collect func(string)
		collect = func(k string) {
			if component[k] {
				return
			}
			component[k] = true
			seen[k] = true
			for next := range neighbors[k] {
				collect(next)
			}
		}
		collect(root)
		// Stable topological order, breaking cycles by URL when no root remains.
		done := map[string]bool{}
		for position := 1; position <= len(component); position++ {
			candidate := ""
			for _, k := range keys {
				if !component[k] || done[k] {
					continue
				}
				ready := true
				for parent := range parents[k] {
					if !done[parent] {
						ready = false
						break
					}
				}
				if ready {
					candidate = k
					break
				}
			}
			if candidate == "" {
				for _, k := range keys {
					if component[k] && !done[k] {
						candidate = k
						break
					}
				}
			}
			done[candidate] = true
			result[candidate] = StackPosition{position, len(component)}
		}
	}
	return result
}
