package tui

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/snooze"
)

// Expand through the selected/owning tasks and their references, then one
// non-task relation. A shared PR must not pull in unrelated sibling tasks.
func (m *Model) snoozeContext(roots []model.TaskReference) []model.TaskReference {
	refs := map[string]model.TaskReference{}
	add := func(r model.TaskReference) {
		r.SnoozeID = ""
		if refSnoozeID(r) != "" {
			refs[refID(r)] = r
		}
	}
	var tasks []model.TaskReference
	for _, r := range roots {
		add(r)
		if r.Provider == "task" {
			tasks = append(tasks, r)
		}
	}
	if len(tasks) == 0 {
		tasks = m.relatedReferences(roots, "task")
	}
	for _, r := range tasks {
		add(r)
		for _, task := range m.tasks.items {
			if task.ID == r.Key {
				for _, ref := range task.References {
					if ref.Provider != "task" {
						add(ref)
					}
				}
			}
		}
	}
	base := sortedRefs(refs)
	for _, r := range m.relatedReferenceKinds(base, "github", "jira", "copilot", "mattermost") {
		add(r)
	}
	return sortedRefs(refs)
}

func (m *Model) expandSnooze() {
	s := m.snoozes
	if s.whole || s.digits != "" || len(s.origins) == 0 {
		s.notice = "Bruk zz før tallet for hele sammenhengen"
		return
	}
	if m.tasks.loadFailed || (m.entities != nil && m.entities.err != nil) {
		s.notice = "Kunne ikke lese oppgaver eller koblinger; avbryt og rett lageret"
		return
	}
	roots := append([]model.TaskReference(nil), s.origins...)
	for i := range roots {
		roots[i].SnoozeID = ""
	}
	s.target = snooze.Entry{ID: "context:" + refSnoozeID(roots[0]), Kind: roots[0].Provider, URL: roots[0].URL, Title: roots[0].Title, Roots: roots, Members: m.snoozeContext(roots)}
	s.targets = []snooze.Entry{s.target}
	s.whole, s.notice = true, ""
}

// Persist newly discovered members before hiding them. Keep known snapshots
// when a provider is unavailable, so refresh/restart cannot undo the pause.
func (m *Model) refreshSnoozeContexts() {
	s := m.snoozes
	if s == nil || !s.contextDirty {
		return
	}
	if m.localIO.busy() {
		return
	}
	s.contextDirty = false
	var changed []snooze.Entry
	for _, e := range s.rows() {
		if len(e.Roots) == 0 {
			continue
		}
		members := map[string]model.TaskReference{}
		for _, r := range e.Members {
			members[refID(r)] = r
		}
		for _, r := range m.snoozeContext(e.Roots) {
			members[refID(r)] = r
		}
		next := sortedRefs(members)
		if !reflect.DeepEqual(next, e.Members) {
			e.Members = next
			changed = append(changed, e)
		}
	}
	if len(changed) > 0 {
		anchor := m.snoozeAnchor()
		m.queueSnooze(changed, false, func(m *Model, err error) {
			if err != nil {
				m.snoozes.contextDirty = true
				m.snoozes.notice = "Kunne ikke utvide snooze: " + err.Error()
				return
			}
			m.snoozes.notice = ""
			m.restoreSnoozeAnchor(anchor)
		})
	}
}

func contextSummary(refs []model.TaskReference) string {
	counts := map[string]int{}
	for _, r := range refs {
		counts[r.Provider]++
	}
	var parts []string
	for _, kind := range []string{"task", "jira", "github", "copilot", "mattermost"} {
		if counts[kind] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[kind], map[string]string{"task": "oppgaver", "jira": "Jira-saker", "github": "PR-er", "copilot": "AI-sesjoner", "mattermost": "chatmeldinger"}[kind]))
		}
	}
	return strings.Join(parts, " · ")
}
