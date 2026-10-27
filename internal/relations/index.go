// Package relations resolves direct entity relations without materializing Jira cliques.
package relations

import (
	"reflect"
	"regexp"
	"slices"
	"sort"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

// Document contains searchable domain metadata and explicit outgoing references.
type Document struct {
	Reference          model.TaskReference
	Text               string
	Repository, Branch string
	Links              []model.TaskReference
}

// Options supplies provider URL rendering without coupling the service to a connector.
// JiraIdentity must change when the renderer's server or context path changes.
type Options struct {
	Pattern      *regexp.Regexp
	JiraIdentity string
	JiraURL      func(string) string
}

type branch struct{ repository, name string }
type urlNode struct {
	children map[byte]*urlNode
	ids      []string
}

// Index owns an immutable snapshot. Sync replaces it only when source data changes.
// Callers must synchronize access; the terminal's event loop is single threaded.
type Index struct {
	jiraIdentity   string
	jiraEnabled    bool
	documents      []Document
	pattern        string
	refs           map[string]model.TaskReference
	keys           map[string][]string
	entityKeys     map[string][]string
	branches       map[branch][]string
	entityBranches map[string]branch
	adjacent       map[string]map[string]bool
	jira           map[string]model.TaskReference
}

func id(r model.TaskReference) string { return r.Provider + ":" + r.URL }

// Sync updates all indexes atomically, including removing obsolete relations.
// Unchanged data reuses the existing indexes. Input slices are copied so in-place
// source edits cannot silently mutate the indexed snapshot.
func (x *Index) Sync(documents []Document, options Options) {
	pattern := options.Pattern
	p := ""
	if pattern != nil {
		p = pattern.String()
	}
	if x.refs != nil && x.pattern == p && reflect.DeepEqual(x.documents, documents) && x.jiraIdentity == options.JiraIdentity && x.jiraEnabled == (options.JiraURL != nil) {
		return
	}
	next := Index{pattern: p, refs: map[string]model.TaskReference{}, keys: map[string][]string{}, entityKeys: map[string][]string{}, branches: map[branch][]string{}, entityBranches: map[string]branch{}, adjacent: map[string]map[string]bool{}, jira: map[string]model.TaskReference{}}

	next.jiraIdentity = options.JiraIdentity
	next.jiraEnabled = options.JiraURL != nil
	next.documents = make([]Document, len(documents))
	root := &urlNode{}
	for i, d := range documents {
		next.documents[i] = d
		next.documents[i].Links = slices.Clone(d.Links)
		r := d.Reference
		if r.URL == "" {
			continue
		}
		rid := id(r)
		next.refs[rid] = r
		if r.Provider == "github" {
			node := root
			for j := 0; j < len(r.URL); j++ {
				if node.children == nil {
					node.children = map[byte]*urlNode{}
				}
				if node.children[r.URL[j]] == nil {
					node.children[r.URL[j]] = &urlNode{}
				}
				node = node.children[r.URL[j]]
			}
			node.ids = append(node.ids, rid)
		}
	}
	edge := func(a, b string) {
		if next.adjacent[a] == nil {
			next.adjacent[a] = map[string]bool{}
		}
		if next.adjacent[b] == nil {
			next.adjacent[b] = map[string]bool{}
		}
		next.adjacent[a][b] = true
		next.adjacent[b][a] = true
	}
	for _, d := range documents {
		r := d.Reference
		if r.URL == "" {
			continue
		}
		rid := id(r)
		keys := (model.PullRequest{Title: r.Key + " " + r.Title + " " + d.Text}).JiraKeys(pattern)
		next.entityKeys[rid] = keys
		for _, k := range keys {
			if options.JiraURL != nil {
				next.jira[k] = model.TaskReference{Provider: "jira", URL: options.JiraURL(k), Key: k, Title: k}
				if known, ok := next.refs[id(next.jira[k])]; ok {
					next.jira[k] = known
				}
			}
			next.keys[k] = append(next.keys[k], rid)
		}
		if d.Branch != "" && (r.Provider == "github" || r.Provider == "copilot") {
			b := branch{d.Repository, d.Branch}
			next.entityBranches[rid] = b
			next.branches[b] = append(next.branches[b], rid)
		}
		for _, other := range d.Links {
			if other.URL == "" {
				continue
			}
			oid := id(other)
			if other.Provider == "task" {
				if _, ok := next.refs[oid]; !ok {
					continue
				}
			}
			if _, ok := next.refs[oid]; !ok {
				next.refs[oid] = other
			}
			edge(rid, oid)
		}
		if r.Provider == "mattermost" {
			text := r.Key + " " + r.Title + " " + d.Text
			for start := 0; start < len(text); start++ {
				node := root
				for j := start; j < len(text); j++ {
					node = node.children[text[j]]
					if node == nil {
						break
					}
					for _, oid := range node.ids {
						edge(rid, oid)
					}
				}
			}
		}
	}
	*x = next
}

// Related follows exactly one relation from each origin, never a connected component.
func (x *Index) Related(origins []model.TaskReference, kinds ...string) []model.TaskReference {
	include := map[string]bool{}
	for _, k := range kinds {
		include[k] = true
	}
	result := map[string]model.TaskReference{}
	add := func(r model.TaskReference) {
		if include[r.Provider] && r.URL != "" {
			r.SnoozeID = ""
			result[id(r)] = r
		}
	}
	for _, origin := range origins {
		rid := id(origin)
		add(origin)
		for oid := range x.adjacent[rid] {
			add(x.refs[oid])
		}
		for _, k := range x.entityKeys[rid] {
			for _, oid := range x.keys[k] {
				if oid != rid {
					add(x.refs[oid])
				}
			}
			add(x.jira[k])
		}
		// Generated Jira references also have the reverse relation.
		if origin.Provider == "jira" {
			if r, ok := x.jira[origin.Key]; ok && id(r) == rid {
				for _, oid := range x.keys[origin.Key] {
					add(x.refs[oid])
				}
			}
		}
		if b, ok := x.entityBranches[rid]; ok {
			for _, oid := range x.branches[b] {
				r := x.refs[oid]
				if r.Provider != origin.Provider {
					add(r)
				}
			}
		}
	}
	rows := make([]model.TaskReference, 0, len(result))
	for _, r := range result {
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Provider == rows[j].Provider {
			return rows[i].URL < rows[j].URL
		}
		return rows[i].Provider < rows[j].Provider
	})
	return rows
}
