package relations

import (
	"fmt"
	"reflect"
	"regexp"
	"testing"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func ref(provider, key string) model.TaskReference {
	return model.TaskReference{Provider: provider, URL: provider + "://" + key, Key: key}
}
func TestDirectRelations(t *testing.T) {
	pr, other, session, post, task, issue := ref("github", "pr"), ref("github", "other"), ref("copilot", "session"), ref("mattermost", "post"), ref("task", "task"), ref("jira", "APP-1")
	docs := []Document{
		{Reference: pr, Text: "APP-1", Repository: "repo", Branch: "branch"},
		{Reference: other},
		{Reference: session, Repository: "repo", Branch: "branch"},
		{Reference: post, Text: "see (" + pr.URL + ")"},
		{Reference: task, Links: []model.TaskReference{pr, other}},
		{Reference: issue},
	}
	var x Index
	x.Sync(docs, Options{JiraURL: func(key string) string { return "jira://" + key }})
	for _, tc := range []struct {
		origin model.TaskReference
		kind   string
		want   []model.TaskReference
	}{
		{pr, "github", []model.TaskReference{pr}}, // Do not traverse through the task.
		{pr, "copilot", []model.TaskReference{session}},
		{session, "github", []model.TaskReference{pr}},
		{pr, "mattermost", []model.TaskReference{post}},
		{post, "github", []model.TaskReference{pr}},
		{pr, "jira", []model.TaskReference{issue}},
		{issue, "github", []model.TaskReference{pr}},
		{pr, "task", []model.TaskReference{task}},
	} {
		if got := x.Related([]model.TaskReference{tc.origin}, tc.kind); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s -> %s: got %v want %v", tc.origin.Provider, tc.kind, got, tc.want)
		}
	}
}
func TestSyncEditsAndRemovals(t *testing.T) {
	a, b := ref("github", "a"), ref("copilot", "b")
	docs := []Document{{Reference: a, Text: "APP-1"}, {Reference: b, Text: "APP-1"}}
	var x Index
	x.Sync(docs, Options{})
	if len(x.Related([]model.TaskReference{a}, "copilot")) != 1 {
		t.Fatal("missing key relation")
	}
	before := reflect.ValueOf(x.refs).Pointer()
	x.Sync(docs, Options{})
	if reflect.ValueOf(x.refs).Pointer() != before {
		t.Fatal("unchanged data rebuilt indexes")
	}
	docs[1].Text = "APP-2"
	x.Sync(docs, Options{})
	if len(x.Related([]model.TaskReference{a}, "copilot")) != 0 {
		t.Fatal("stale key relation")
	}
	docs[0].Links = []model.TaskReference{b}
	x.Sync(docs, Options{})
	if len(x.Related([]model.TaskReference{a}, "copilot")) != 1 {
		t.Fatal("missing explicit relation")
	}
	docs[0].Links[0] = ref("task", "deleted")
	x.Sync(docs, Options{})
	if len(x.Related([]model.TaskReference{a}, "copilot", "task")) != 0 {
		t.Fatal("stale or deleted task relation")
	}
	docs = docs[:1]
	x.Sync(docs, Options{})
	if _, ok := x.refs[id(b)]; ok {
		t.Fatal("deleted entity retained")
	}
}
func TestCustomPatternSnoozeAndSharedKeys(t *testing.T) {
	a, b, c := ref("github", "a"), ref("copilot", "b"), ref("mattermost", "c")
	docs := []Document{{Reference: a, Text: "OPS.12 OPS.12"}, {Reference: b, Text: "OPS.12"}, {Reference: c, Text: "OPS.123"}}
	var x Index
	x.Sync(docs, Options{Pattern: regexp.MustCompile(`OPS\.[0-9]+`)})
	a.SnoozeID = "group"
	if got := x.Related([]model.TaskReference{a, a}, "copilot"); len(got) != 1 || got[0] != b {
		t.Fatalf("shared key/snooze: %v", got)
	}
	if got := x.Related([]model.TaskReference{a}, "mattermost"); len(got) != 0 {
		t.Fatal("partial key match")
	}
	x.Sync(docs, Options{Pattern: regexp.MustCompile(`APP-[0-9]+`)})
	if len(x.Related([]model.TaskReference{a}, "copilot")) != 0 {
		t.Fatal("pattern update retained old key")
	}
}
func benchmarkDocuments(n int) []Document {
	docs := make([]Document, 0, n*3)
	for i := 0; i < n; i++ {
		pr := ref("github", fmt.Sprintf("pr/%d", i))
		docs = append(docs, Document{Reference: pr, Text: fmt.Sprintf("APP-%d", i/20), Repository: "repo", Branch: fmt.Sprint(i)}, Document{Reference: ref("copilot", fmt.Sprint(i)), Repository: "repo", Branch: fmt.Sprint(i)}, Document{Reference: ref("mattermost", fmt.Sprint(i)), Text: "see " + pr.URL})
	}
	return docs
}
func BenchmarkRelations(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			docs := benchmarkDocuments(n)
			origins := []model.TaskReference{docs[0].Reference}
			b.Run("Build", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					var x Index
					x.Sync(docs, Options{})
				}
			})
			var x Index
			x.Sync(docs, Options{})
			b.Run("Lookup", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					x.Related(origins, "github", "copilot", "mattermost")
				}
			})
			b.Run("UnchangedSyncAndLookup", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					x.Sync(docs, Options{})
					x.Related(origins, "github", "copilot", "mattermost")
				}
			})
		})
	}
}

func TestSyncProviderMetadata(t *testing.T) {
	pr, session, post := ref("github", "pr"), ref("copilot", "session"), ref("mattermost", "post")
	docs := []Document{{Reference: pr, Repository: "repo", Branch: "old"}, {Reference: session, Repository: "repo", Branch: "old"}, {Reference: post, Text: pr.URL}}
	var x Index
	check := func(kind string, count int) {
		t.Helper()
		if got := x.Related([]model.TaskReference{pr}, kind); len(got) != count {
			t.Fatalf("%s: got %v", kind, got)
		}
	}
	x.Sync(docs, Options{})
	check("copilot", 1)
	check("mattermost", 1)
	docs[1].Repository = "different"
	x.Sync(docs, Options{})
	check("copilot", 0)
	docs[1].Repository = "repo"
	docs[1].Branch = "new"
	x.Sync(docs, Options{})
	check("copilot", 0)
	docs[0].Branch = "new"
	x.Sync(docs, Options{})
	check("copilot", 1)
	docs[2].Text = "no URL"
	x.Sync(docs, Options{})
	check("mattermost", 0)
	docs[0].Text = "APP-1"
	for _, server := range []string{"https://first/", "https://second/context/"} {
		options := Options{JiraIdentity: server, JiraURL: func(key string) string { return server + key }}
		x.Sync(docs, options)
		got := x.Related([]model.TaskReference{pr}, "jira")
		if len(got) != 1 || got[0].URL != server+"APP-1" {
			t.Fatalf("stale Jira server: %v", got)
		}
	}
	x.Sync(docs, Options{})
	check("jira", 0)
}

func TestSharedKeyStorageIsLinear(t *testing.T) {
	docs := make([]Document, 10000)
	for i := range docs {
		docs[i] = Document{Reference: ref("github", fmt.Sprint(i)), Text: "APP-1"}
	}
	var x Index
	x.Sync(docs, Options{})
	if len(x.keys["APP-1"]) != len(docs) || len(x.adjacent) != 0 {
		t.Fatal("shared key materialized pairwise edges")
	}
	if got := x.Related([]model.TaskReference{docs[0].Reference}, "github"); len(got) != len(docs) {
		t.Fatalf("missing direct shared-key relations: %d", len(got))
	}
}
