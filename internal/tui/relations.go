package tui

import (
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/relations"
)

// Adapt provider snapshots to domain documents. Sync detects source changes,
// including in-place edits, and keeps unchanged indexes for later queries.
func (m *Model) relatedReferenceKinds(origins []model.TaskReference, kinds ...string) []model.TaskReference {
	if m.relations == nil {
		m.relations = &relations.Index{}
	}
	refs := m.allReferences()
	documents := make([]relations.Document, len(refs))
	positions := make(map[string]int, len(refs))
	for i, r := range refs {
		documents[i].Reference = r
		positions[refID(r)] = i
	}
	for _, dataset := range m.prs.Snapshot().Datasets {
		for _, pr := range dataset.PRs {
			if i, ok := positions[refID(prRef(pr))]; ok {
				documents[i].Text += " " + pr.Body + " " + pr.HeadRef
				documents[i].Repository, documents[i].Branch = pr.Repository, pr.HeadRef
			}
		}
	}
	if m.copilot != nil {
		for _, session := range m.copilot.sessions {
			if i, ok := positions[refID(copilotRef(session))]; ok {
				documents[i].Text += " " + session.Jira + " " + session.Branch
				documents[i].Repository, documents[i].Branch = session.Repository, session.Branch
			}
		}
	}
	for _, task := range m.tasks.items {
		if i, ok := positions[refID(taskRef(task))]; ok {
			documents[i].Links = append(documents[i].Links, task.References...)
		}
	}
	if m.entities != nil {
		for _, link := range m.entities.data.Links {
			if link.Left.Provider == "task" {
				if _, ok := positions[refID(link.Left)]; !ok {
					continue
				}
			}
			if link.Right.Provider == "task" {
				if _, ok := positions[refID(link.Right)]; !ok {
					continue
				}
			}
			if i, ok := positions[refID(link.Left)]; ok {
				documents[i].Links = append(documents[i].Links, link.Right)
			}
		}
	}
	options := relations.Options{Pattern: m.jiraPattern}
	if m.jira != nil && m.jira.client != nil {
		options.JiraURL = m.jira.client.IssueURL
		options.JiraIdentity = m.jira.client.IssueURL("")
	}
	m.relations.Sync(documents, options)
	return m.relations.Related(origins, kinds...)
}
