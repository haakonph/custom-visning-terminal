package tui

import (
	"fmt"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"reflect"
	"strings"
	"testing"
)

func (m *Model) legacyRelatedReferenceKinds(origins []model.TaskReference, kinds ...string) []model.TaskReference {
	// Deferred rows have their own selection identity, but relations use the
	// underlying provider entity, including for separately deferred groups.
	origins = underlyingReferences(origins)
	include := map[string]bool{}
	for _, kind := range kinds {
		include[kind] = true
	}
	refs := map[string]model.TaskReference{}
	adj := map[string][]string{}
	edge := func(a, b model.TaskReference) {
		if a.URL == "" || b.URL == "" {
			return
		}
		if _, ok := refs[refID(a)]; !ok {
			refs[refID(a)] = a
		}
		if _, ok := refs[refID(b)]; !ok {
			refs[refID(b)] = b
		}
		adj[refID(a)] = append(adj[refID(a)], refID(b))
		adj[refID(b)] = append(adj[refID(b)], refID(a))
	}
	all := m.allReferences()
	for _, r := range all {
		refs[refID(r)] = r
	}
	for _, r := range origins {
		refs[refID(r)] = r
	}
	if m.entities != nil {
		for _, e := range m.entities.data.Links {
			if e.Left.Provider == "task" {
				if _, exists := refs[refID(e.Left)]; !exists {
					continue
				}
			}
			if e.Right.Provider == "task" {
				if _, exists := refs[refID(e.Right)]; !exists {
					continue
				}
			}
			edge(e.Left, e.Right)
		}
	}
	for _, task := range m.tasks.items {
		for _, r := range task.References {
			edge(taskRef(task), r)
		}
	}
	keyEntities := map[string][]model.TaskReference{}
	for _, r := range all {
		text := r.Key + " " + r.Title
		if r.Provider == "copilot" && m.copilot != nil {
			for _, s := range m.copilot.sessions {
				if s.ID == r.Key {
					text += " " + s.Jira + " " + s.Branch
					for _, d := range m.prs.Snapshot().Datasets {
						for _, p := range d.PRs {
							if s.Repository == p.Repository && s.Branch != "" && s.Branch == p.HeadRef {
								edge(r, prRef(p))
							}
						}
					}
				}
			}
		}
		if r.Provider == "github" {
			for _, d := range m.prs.Snapshot().Datasets {
				for _, p := range d.PRs {
					if p.URL == r.URL {
						text += " " + p.Body + " " + p.HeadRef
					}
				}
			}
		}
		p := model.PullRequest{Title: text}
		for _, key := range p.JiraKeys(m.jiraPattern) {
			keyEntities[key] = append(keyEntities[key], r)
			if m.jira != nil {
				edge(r, jiraRef(key, *m))
			}
		}
		if r.Provider == "mattermost" {
			for _, other := range all {
				if other.Provider == "github" && strings.Contains(text, other.URL) {
					edge(r, other)
				}
			}
		}
	}
	for _, group := range keyEntities {
		for _, a := range group {
			for _, b := range group {
				if refID(a) != refID(b) {
					edge(a, b)
				}
			}
		}
	}
	// Each typed thing traverses one relation. Never expand an action to an
	// entire connected component through unrelated tasks.
	result := map[string]model.TaskReference{}
	for _, origin := range origins {
		id := refID(origin)
		if include[origin.Provider] {
			result[id] = origin
		}
		for _, neighbor := range adj[id] {
			r := refs[neighbor]
			if include[r.Provider] {
				result[neighbor] = r
			}
		}
	}
	return sortedRefs(result)
}
func BenchmarkRelationCalculation(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			m := Model{entities: &entityState{}, copilot: &copilotState{}, mmReader: &mattermostReader{}}
			m.mmReader.data.Server = "https://chat.example"
			for i := 0; i < n; i++ {
				pr := model.PullRequest{URL: fmt.Sprintf("https://github.com/acme/api/pull/%d", i+1), Repository: "acme/api", Number: i + 1, Title: fmt.Sprintf("APP-%d work", i/20), HeadRef: fmt.Sprintf("branch-%d", i)}
				m.editPRs(func(snapshot *model.Snapshot) { snapshot.Datasets[0].PRs = append(snapshot.Datasets[0].PRs, pr) })
				m.copilot.sessions = append(m.copilot.sessions, model.CopilotSession{ID: fmt.Sprint(i), Name: "session", Repository: pr.Repository, Branch: pr.HeadRef})
				m.mmReader.data.Posts = append(m.mmReader.data.Posts, model.MattermostPost{ID: fmt.Sprint(i), Message: "See " + pr.URL})
			}
			origins := []model.TaskReference{prRef(m.prs.Snapshot().Datasets[0].PRs[0])}
			b.Run("Legacy", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					m.legacyRelatedReferenceKinds(origins, "github", "copilot", "mattermost")
				}
			})
			b.Run("BuildAndLookup", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					m.relations = nil
					m.relatedReferenceKinds(origins, "github", "copilot", "mattermost")
				}
			})
			b.Run("Indexed", func(b *testing.B) {
				m.relatedReferenceKinds(origins, "github", "copilot", "mattermost")
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					m.relatedReferenceKinds(origins, "github", "copilot", "mattermost")
				}
			})
		})
	}
}
func TestRelationIndexMatchesLegacy(t *testing.T) {
	m := unifiedModel(t)
	for _, origin := range m.allReferences() {
		for _, kind := range []string{"task", "github", "jira", "copilot", "mattermost"} {
			want := m.legacyRelatedReferenceKinds([]model.TaskReference{origin}, kind)
			got := m.relatedReferenceKinds([]model.TaskReference{origin}, kind)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%v -> %s: got %v want %v", origin, kind, got, want)
			}
		}
	}
}

func TestRelationIndexTracksDataChanges(t *testing.T) {
	m := unifiedModel(t)
	compare := func() {
		t.Helper()
		for _, origin := range m.allReferences() {
			got := m.relatedReferenceKinds([]model.TaskReference{origin}, "github", "jira", "copilot", "mattermost", "task")
			want := m.legacyRelatedReferenceKinds([]model.TaskReference{origin}, "github", "jira", "copilot", "mattermost", "task")
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%s: got %v want %v", origin.URL, got, want)
			}
		}
	}
	compare()
	m.editPRs(func(snapshot *model.Snapshot) { snapshot.Datasets[0].PRs[0].Body = "APP-2" })
	compare()
	m.copilot.sessions[0].Jira = ""
	m.copilot.sessions[0].Branch = "unrelated"
	compare()
	m.tasks.items[0].References = nil
	compare()
	m.entities.data.Links = []model.EntityLink{{Left: copilotRef(m.copilot.sessions[0]), Right: prRef(m.prs.Snapshot().Datasets[0].PRs[1])}}
	compare()
	m.entities.data.Links = nil
	compare()
	m.jira.rows = nil
	compare()
	m.copilot.sessions = m.copilot.sessions[1:]
	compare()
	if len(m.mmReader.data.Posts) > 0 {
		m.mmReader.data.Posts[0].DeletedAt = 1
		compare()
	}
}
