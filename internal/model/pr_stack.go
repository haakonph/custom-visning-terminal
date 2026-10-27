package model

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
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

	// Resolve references through indexes instead of comparing every pair of PRs.
	// Repository identity includes the URL prefix, so hosts remain isolated.
	type repositoryID struct{ name, prefix string }
	repositories := map[string]repositoryID{}
	byURL := map[string][]string{}
	byNumber := map[repositoryID]map[int][]string{}
	byHead := map[repositoryID]map[string][]string{}
	for _, k := range keys {
		p := byKey[k]
		prefix, _, _ := strings.Cut(p.URL, "/pull/")
		repo := repositoryID{stackIdentity(p.Repository), stackIdentity(prefix)}
		repositories[k] = repo
		byURL[p.URL] = append(byURL[p.URL], k)
		if byNumber[repo] == nil {
			byNumber[repo] = map[int][]string{}
			byHead[repo] = map[string][]string{}
		}
		byNumber[repo][p.Number] = append(byNumber[repo][p.Number], k)
		if p.HeadRef != "" {
			byHead[repo][p.HeadRef] = append(byHead[repo][p.HeadRef], k)
		}
	}
	for _, k := range keys {
		p, repo := byKey[k], repositories[k]
		for _, url := range prLink.FindAllString(p.Body, -1) {
			for _, parent := range byURL[url] {
				link(k, parent)
			}
		}
		for _, match := range prNumber.FindAllStringSubmatch(prLink.ReplaceAllString(p.Body, ""), -1) {
			number, _ := strconv.Atoi(match[1])
			for _, parent := range byNumber[repo][number] {
				link(k, parent)
			}
		}
		if p.BaseRef != "" {
			for _, parent := range byHead[repo][p.BaseRef] {
				link(k, parent)
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

// Use the same Unicode equivalence as EqualFold when indexing repository names
// and URL prefixes. Branch names and explicit body URLs remain case sensitive.
func stackIdentity(value string) string {
	return strings.Map(func(r rune) rune {
		first := r
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			if next < first {
				first = next
			}
		}
		return first
	}, value)
}
