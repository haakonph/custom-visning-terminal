package tui

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func groupedFixture() []model.PullRequest {
	return []model.PullRequest{
		{URL: "https://github.com/acme/api/pull/1", Number: 1, Title: "ABC-1 API", Repository: "acme/api"},
		{URL: "https://github.com/acme/docs/pull/2", Number: 2, Title: "Unlinked", Repository: "acme/docs"},
		{URL: "https://github.com/acme/web/pull/3", Number: 3, Title: "XYZ-2 Web", Repository: "acme/web"},
		{URL: "https://github.com/acme/web/pull/4", Number: 4, Title: "ABC-1 XYZ-2 Tests", Repository: "acme/web"},
	}
}

func TestPRGroupingAcrossViews(t *testing.T) {
	m := ready(t)
	for view := model.Open; view < model.ViewCount; view++ {
		t.Run(view.String(), func(t *testing.T) {
			m.active = view
			m.snapshot.Datasets[view].PRs = groupedFixture()
			rows := m.filtered()
			var numbers []int
			for _, p := range rows {
				numbers = append(numbers, p.Number)
			}
			if !reflect.DeepEqual(numbers, []int{1, 4, 3, 2}) {
				t.Fatal(numbers)
			}
			if m.snapshot.Datasets[view].PRs[1].Number != 2 {
				t.Fatal("grouping changed source order")
			}
			m.height, m.width, m.selected = 40, 160, 0
			text := ansi.Strip(m.View().Content)
			for _, want := range []string{"▼ ABC-1 · 2 PR", "▼ XYZ-2 · 1 PR", "▼ Uten Jira-sak · 1 PR"} {
				if !strings.Contains(text, want) {
					t.Fatalf("missing %q in %s", want, text)
				}
			}
			m, _ = update(m, key('j'))
			if refs := m.currentReferences(); len(refs) != 1 || refs[0].URL != rows[1].URL {
				t.Fatal("navigation selected a different PR", refs)
			}
			m.input.SetValue("Tests")
			if rows := m.filtered(); len(rows) != 1 || rows[0].Number != 4 {
				t.Fatal(rows)
			}
			m.input.SetValue("")
		})
	}
}

func TestPRGroupingKeySourcesAndRelatedIssue(t *testing.T) {
	m := ready(t)
	for _, p := range []model.PullRequest{{Title: "ABC-7 title"}, {Body: "ABC-7"}, {HeadRef: "fix/ABC-7"}} {
		if got := m.prGroupKey(p); got != "ABC-7" {
			t.Fatal(got)
		}
	}
	m.jiraPattern = regexp.MustCompile(`CUSTOM-[0-9]{4}`)
	if got := m.prGroupKey(model.PullRequest{Title: "ABC-7 CUSTOM-1234"}); got != "CUSTOM-1234" {
		t.Fatal(got)
	}
	m.relatedKey = "XYZ-2"
	if got := m.prGroupKey(model.PullRequest{Title: "ABC-1 XYZ-2"}); got != "XYZ-2" {
		t.Fatal(got)
	}
}

func TestDependabotReviewGroupIsLast(t *testing.T) {
	for _, view := range []model.View{model.Reviews, model.TeamReviews} {
		t.Run(view.String(), func(t *testing.T) {
			m := ready(t)
			m.active = view
			rows := groupedFixture()
			rows[0].Author = "dependabot[bot]" // Even a Jira-linked bot PR belongs at the end.
			rows[2].Author = "DEPENDABOT"      // GraphQL login and case variants also match.
			rows = append(rows, model.PullRequest{Number: 5, URL: "human", Author: "dependabot-helper", Title: "Unlinked human PR"})
			rows = append(rows, model.PullRequest{Number: 6, URL: "bot", Author: "dependabot[bot]", Title: "Update dependency"})
			m.snapshot.Datasets[view].PRs = rows
			var numbers []int
			for _, p := range m.filtered() {
				numbers = append(numbers, p.Number)
			}
			if !reflect.DeepEqual(numbers, []int{4, 2, 5, 1, 3, 6}) {
				t.Fatal(numbers)
			}
			if rows[0].Number != 1 {
				t.Fatal("grouping changed cached order")
			}
			m.height, m.width = 40, 160
			text := ansi.Strip(m.View().Content)
			if strings.Count(text, "▼ Dependabot · 3 PR") != 1 || strings.Index(text, "▼ Dependabot") < strings.Index(text, "▼ Uten Jira-sak") {
				t.Fatalf("missing final Dependabot group: %s", text)
			}
			m.selected = 3
			if refs := m.currentReferences(); len(refs) != 1 || refs[0].URL != rows[0].URL {
				t.Fatal("selected a different PR", refs)
			}
			m.input.SetValue("XYZ-2")
			filtered := m.filtered()
			if len(filtered) != 2 || filtered[0].Number != 4 || filtered[1].Number != 3 {
				t.Fatal(filtered)
			}
		})
	}
}

func TestDependabotGroupingOnlyAppliesToReviewViews(t *testing.T) {
	m := ready(t)
	p := model.PullRequest{Author: "dependabot[bot]", Title: "ABC-1 Update dependency"}
	for _, view := range []model.View{model.Open, model.Drafts} {
		m.active = view
		if got := m.prGroupKey(p); got != "ABC-1" {
			t.Fatalf("%s: %q", view, got)
		}
	}
	m.active, m.relatedKey = model.Reviews, "XYZ-2"
	if got := m.prGroupKey(p); got != dependabotGroup {
		t.Fatal(got)
	}
}

func TestGroupedPRViewportKeepsSelectedRow(t *testing.T) {
	m := ready(t)
	m.width = 100
	rows := m.groupPRs(groupedFixture())
	for capacity := 1; capacity <= 8; capacity++ {
		for selected := range rows {
			m.selected = selected
			lines := m.groupedPRLines(rows, capacity)
			text := ansi.Strip(strings.Join(lines, "\n"))
			if len(lines) > capacity || !strings.Contains(text, fmt.Sprintf("#%d ", rows[selected].Number)) {
				t.Fatalf("capacity=%d selected=%d: %s", capacity, selected, text)
			}
			for _, line := range lines {
				if ansi.StringWidth(line) > m.width {
					t.Fatalf("line too wide: %s", line)
				}
			}
			if capacity >= 2 && !strings.Contains(text, "▼ "+m.prGroupKey(rows[selected])) && m.prGroupKey(rows[selected]) != "" {
				t.Fatalf("missing group context: %s", text)
			}
		}
	}
}

func TestActivateGroupedPRUsesVisibleOrder(t *testing.T) {
	m := ready(t)
	m.snapshot.Datasets[model.Open].PRs = groupedFixture()
	next, _ := m.activateReference(prRef(groupedFixture()[3]))
	m = next.(Model)
	if rows := m.filtered(); rows[m.selected].Number != 4 {
		t.Fatal("activated a different PR")
	}
}
