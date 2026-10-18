package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/storage/dbgen"
)

func TestMigrationKeepsBackupAndDatabaseWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	original := `[{"id":"stable","title":"saved","references":[{"provider":"jira","key":"ABC-1","url":"https://jira.example/ABC-1"}]}]`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	var value []model.Task
	if err := Load(path, &value); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(path, value); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(Path(path))
	if err != nil || len(data) < 16 || string(data[:16]) != "SQLite format 3\x00" {
		t.Fatalf("not SQLite: %v", err)
	}
	backup, _ := os.ReadFile(path)
	if string(backup) != original {
		t.Fatal("legacy backup modified")
	}
	value[0].Title = "updated"
	if err := Save(path, value); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(path, []model.Task{{Title: "stale"}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("broken stale backup"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Load(path, &value); err != nil || value[0].Title != "updated" || value[0].References[0].Key != "ABC-1" {
		t.Fatal(value, err)
	}
	info, _ := os.Stat(Path(path))
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	db, err := open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := dbgen.New(db).ListTasks(context.Background())
	if err != nil || len(rows) != 1 || rows[0].StableID != "stable" {
		t.Fatal(rows, err)
	}
}

func TestMissingCorruptAndFailedWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	var value []model.Task
	if err := Load(path, &value); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	for _, input := range []string{"broken", `[] {}`} {
		if err := os.WriteFile(path, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		if err := Load(path, &value); err == nil {
			t.Fatal("accepted corrupt legacy JSON")
		}
		if _, err := os.Stat(Path(path)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("created database before validation")
		}
	}
	if err := Save(path, []model.Task{{Title: "keep"}}); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, make(chan int)); err == nil {
		t.Fatal("accepted unsupported type")
	}
	if err := Load(path, &value); err != nil || value[0].Title != "keep" {
		t.Fatal(value, err)
	}
	if err := os.WriteFile(Path(path), []byte("broken database"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Load(path, &value); err == nil {
		t.Fatal("fell back from corrupt SQLite")
	}
	data, _ := os.ReadFile(Path(path))
	if string(data) != "broken database" {
		t.Fatal("corrupt database modified")
	}
}

func TestConcurrentSavesAreAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.sqlite")
	if err := Save(path, []model.Task{{ID: "0", Title: "0"}}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			title := time.Duration(n).String()
			if err := Save(path, []model.Task{{ID: title, Title: title}}); err != nil {
				t.Error(err)
				return
			}
			var value []model.Task
			if err := Load(path, &value); err != nil {
				t.Error(err)
				return
			}
			if len(value) != 1 || value[0].ID != value[0].Title {
				t.Error("partial write")
			}
		}(i)
	}
	wg.Wait()
}

func legacyDatabase(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", Path(path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE documents (id INTEGER PRIMARY KEY CHECK (id = 1), payload BLOB NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO documents(id,payload) VALUES(1,?)`, data); err != nil {
		t.Fatal(err)
	}
}

func TestDocumentDatabaseImportsIntoNormalizedTables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.sqlite")
	original := []model.Task{{ID: "stable", Title: "Task", References: []model.TaskReference{{Provider: "github", URL: "https://github.com/o/r/pull/1", Key: "o/r#1", Title: "PR", Role: "review"}}}}
	legacyDatabase(t, path, original)
	var loaded []model.Task
	if err := Load(path, &loaded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(original, loaded) {
		t.Fatal(loaded)
	}
	if err := Migrate(path, loaded); err != nil {
		t.Fatal(err)
	}
	if err := Load(path, &loaded); err != nil || !reflect.DeepEqual(original, loaded) {
		t.Fatal(loaded, err)
	}
	db, err := open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := dbgen.New(db).LoadLegacyDocument(context.Background()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("legacy document not retired", err)
	}
	var version int
	if err := db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil || version != 6 {
		t.Fatal(version, err)
	}
}

func TestFailedConversionRollsBackAndRetainsDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.sqlite")
	original := model.CopilotStore{Version: 1, Sessions: map[string]*model.CopilotStoredSession{}, Traces: map[string]string{"orphan": "missing-session"}}
	legacyDatabase(t, path, original)
	var data model.CopilotStore
	if err := Load(path, &data); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(path, data); err == nil {
		t.Fatal("accepted orphan trace")
	}
	db, err := open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	q := dbgen.New(db)
	if _, err := q.LoadLegacyDocument(context.Background()); err != nil {
		t.Fatal("lost original document", err)
	}
	if _, err := q.GetState(context.Background(), "copilot"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("committed failed conversion", err)
	}
	sessions, err := q.ListCopilotSessions(context.Background())
	if err != nil || len(sessions) != 0 {
		t.Fatal(sessions, err)
	}
}

func TestGitHubDeduplicatesAndPreservesMemberships(t *testing.T) {
	path := filepath.Join(t.TempDir(), "github.sqlite")
	at := time.Now().UTC().Truncate(time.Second)
	pr := model.PullRequest{ID: "node", Repository: "org/repo", Number: 7, URL: "https://github.com/org/repo/pull/7", Title: "Latest", Body: "Body", HeadOID: "commit", HeadRef: "ABC-1", Author: "alice", CreatedAt: at, UpdatedAt: at, Review: model.Approved, CI: model.Pass, Mergeability: model.Mergeable, Labels: []string{"bug", "priority"}, RequestedReviewers: []string{"z", "a"}, RequestedTeams: []string{"org/team"}}
	snap := model.Snapshot{Version: 1, User: "alice"}
	snap.Datasets[model.Open] = model.Dataset{PRs: []model.PullRequest{pr}, SyncedAt: at}
	snap.Datasets[model.Reviews] = snap.Datasets[model.Open]
	if err := Save(path, snap); err != nil {
		t.Fatal(err)
	}
	var got model.Snapshot
	if err := Load(path, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, snap) {
		t.Fatalf("roundtrip: %#v", got)
	}
	db, err := open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	q := dbgen.New(db)
	prs, err := q.ListGithubPullRequests(context.Background())
	if err != nil || len(prs) != 1 {
		t.Fatal(prs, err)
	}
	members, err := q.ListGithubViewMemberships(context.Background())
	if err != nil || len(members) != 2 {
		t.Fatal(members, err)
	}
	var title string
	if err := db.QueryRow("SELECT title FROM overview_items WHERE provider = 'github'").Scan(&title); err != nil || title != "Latest" {
		t.Fatal(title, err)
	}
	if _, err := db.Exec("INSERT INTO github_labels(pull_request_id,position,label) VALUES('missing',0,'orphan')"); err == nil {
		t.Fatal("foreign keys disabled")
	}
}

func TestRejectsFutureSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.sqlite")
	db, err := open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO schema_migrations(version) VALUES(999)"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := Save(path, []model.Task{{Title: "overwrite"}}); err == nil {
		t.Fatal("accepted future schema")
	}
}

func TestJiraDocumentMigrationPreservesFieldsAndOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jira.sqlite")
	at := time.Now().UTC().Truncate(time.Second)
	issue := model.JiraIssue{Key: "ABC-7", URL: "https://jira.example/ABC-7", Title: "Title", Status: "Open", Priority: "High", Assignee: "alice", Project: "ABC", Type: "Bug", Description: "Description", Due: "2026-10-20", Labels: []string{"z", "a"}, Updated: at}
	original := model.JiraCache{Query: "project=ABC ORDER BY priority", Issues: []model.JiraIssue{issue, {Key: "ABC-1", Project: "ABC", Updated: at}}, SyncedAt: at}
	legacyDatabase(t, path, original)
	var loaded model.JiraCache
	if err := Load(path, &loaded); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(path, loaded); err != nil {
		t.Fatal(err)
	}
	if err := Load(path, &loaded); err != nil || !reflect.DeepEqual(original, loaded) {
		t.Fatal(loaded, err)
	}
	db, err := open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	q := dbgen.New(db)
	projects, err := q.ListJiraProjects(context.Background())
	if err != nil || len(projects) != 1 {
		t.Fatal(projects, err)
	}
	issues, err := q.ListJiraIssues(context.Background())
	if err != nil || len(issues) != 2 {
		t.Fatal(issues, err)
	}
}

func TestCopilotPreservesEventIdentityMetadataAndNullableUsage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "copilot.sqlite")
	zero := int64(0)
	duration := 12.5
	event := model.CopilotEvent{SessionID: "original-session", Key: "source-key", Source: "otlp", TraceID: "trace", SpanID: "span", EventID: "evt", StartTime: "2026-10-18T10:00:00Z", State: "completed", Meta: map[string]any{"nested": map[string]any{"arbitrary": "value"}}, MetricSeries: "series", Temporality: 2, Time: "2026-10-18T10:01:00Z", Type: "tool", Content: "content", Role: "assistant", Tool: "shell", CallID: "call", Args: "args", Result: "result", Error: "error", Warning: "warning", Agent: "agent", Parent: "parent", Model: "model", InputTokens: &zero, DurationMs: &duration, Raw: json.RawMessage(`{"custom":true}`)}
	original := model.CopilotStore{Version: 1, Sessions: map[string]*model.CopilotStoredSession{"session": {Meta: map[string]any{"cwd": "/work", "custom": true}, Events: map[string]model.CopilotEvent{"stored-map-key": event}}}, Traces: map[string]string{"trace": "session"}}
	legacyDatabase(t, path, original)
	var loaded model.CopilotStore
	if err := Load(path, &loaded); err != nil {
		t.Fatal(err)
	}
	loaded.Summaries = map[string]model.CopilotSession{"session": {Name: "Summary", Repository: "org/repo", Branch: "ABC-1", Status: "completed", InputTokens: &zero, DurationMs: &duration}}
	if err := Migrate(path, loaded); err != nil {
		t.Fatal(err)
	}
	if err := Load(path, &loaded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(original.Sessions, loaded.Sessions) || !reflect.DeepEqual(original.Traces, loaded.Traces) {
		t.Fatalf("event roundtrip changed: %#v", loaded)
	}
	summary := loaded.Summaries["session"]
	if summary.Name != "Summary" || summary.InputTokens == nil || *summary.InputTokens != 0 || summary.OutputTokens != nil {
		t.Fatal(summary)
	}
	// A failed replacement after deleting old rows must roll back all tables.
	invalid := original
	invalid.Traces = map[string]string{"orphan": "missing"}
	if err := Save(path, invalid); err == nil {
		t.Fatal("accepted orphan trace")
	}
	if err := Load(path, &loaded); err != nil || !reflect.DeepEqual(original.Sessions, loaded.Sessions) || loaded.Summaries["session"].Name != "Summary" {
		t.Fatal("failed write lost prior data", err)
	}
}

func TestEmptySnapshotDoesNotResurrectLegacyTasks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	if err := os.WriteFile(path, []byte(`[{"title":"stale"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, []model.Task{}); err != nil {
		t.Fatal(err)
	}
	var loaded []model.Task
	if err := Load(path, &loaded); err != nil || len(loaded) != 0 {
		t.Fatal(loaded, err)
	}
	if err := Migrate(path, []model.Task{{Title: "stale"}}); err != nil {
		t.Fatal(err)
	}
	if err := Load(path, &loaded); err != nil || len(loaded) != 0 {
		t.Fatal(loaded, err)
	}
}

func TestLatestObservationWinsWhenPRUpdateTimeIsUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "github.sqlite")
	at := time.Now().UTC()
	fresh := model.PullRequest{URL: "https://github.com/o/r/pull/1", UpdatedAt: at, CI: model.Pass}
	stale := fresh
	stale.CI = model.Pending
	snap := model.Snapshot{Version: 1, User: "alice"}
	snap.Datasets[model.Open] = model.Dataset{PRs: []model.PullRequest{fresh}, SyncedAt: at}
	snap.Datasets[model.Reviews] = model.Dataset{PRs: []model.PullRequest{stale}, SyncedAt: at.Add(-time.Hour)}
	if err := Save(path, snap); err != nil {
		t.Fatal(err)
	}
	var loaded model.Snapshot
	if err := Load(path, &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.Datasets[model.Open].PRs[0].CI != model.Pass || loaded.Datasets[model.Reviews].PRs[0].CI != model.Pass {
		t.Fatal("older observation replaced the shared PR")
	}
}
