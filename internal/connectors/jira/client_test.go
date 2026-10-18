package jira

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewTLSServer(h)
	t.Cleanup(server.Close)
	c, err := New(server.URL+"/jira", "secret")
	if err != nil {
		t.Fatal(err)
	}
	c.http = server.Client()
	c.http.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return c
}
func TestSearchPaginatesAndMaps(t *testing.T) {
	calls := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jira/rest/api/2/search" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("wrong request")
		}
		var body struct {
			StartAt int
			JQL     string
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.StartAt != calls || body.JQL != "project = APP ORDER BY updated DESC" {
			t.Error("incorrect pagination/query")
		}
		calls++
		if body.StartAt == 0 {
			w.Write([]byte(`{"startAt":0,"total":2,"issues":[{"key":"APP-1","fields":{"summary":"First","status":{"name":"In Progress"},"priority":{"name":"High"},"assignee":null,"project":{"key":"APP"},"updated":"2026-10-17T10:00:00.000+0200"}}]}`))
		} else {
			w.Write([]byte(`{"startAt":1,"total":2,"issues":[{"key":"APP-2","fields":{"summary":"Second","assignee":{"displayName":"Haakon"}}}]}`))
		}
	})
	rows, err := c.Search(context.Background(), "project = APP ORDER BY updated DESC")
	if err != nil || len(rows) != 2 || calls != 2 {
		t.Fatalf("rows=%v err=%v calls=%d", rows, err, calls)
	}
	if rows[0].Assignee != "Unassigned" || rows[1].Assignee != "Haakon" || rows[0].Updated.IsZero() || !c.ValidIssueURL(rows[0].URL) {
		t.Fatal(rows)
	}
	if c.ValidIssueURL("https://evil.example/browse/APP-1") {
		t.Fatal("external origin accepted")
	}
}
func TestErrorsDoNotExposeResponseOrToken(t *testing.T) {
	for _, status := range []int{400, 401, 403, 429, 500, 302} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				w.Write([]byte("secret sensitive response"))
			})
			_, err := c.Search(context.Background(), "bad query")
			if err == nil || strings.Contains(err.Error(), "secret") {
				t.Fatalf("unsafe/missing error: %v", err)
			}
		})
	}
}
func TestSearchCancellation(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Search(ctx, "x"); err == nil {
		t.Fatal("expected cancellation")
	}
}
func TestFiltersEscapeAndCompose(t *testing.T) {
	q := (Filters{Project: `APP" OR project = BAD`, Status: "In Progress"}).JQL("statusCategory != Done ORDER BY updated DESC")
	expected := `(statusCategory != Done) AND project = "APP\" OR project = BAD" AND status = "In Progress" ORDER BY updated DESC`
	if q != expected {
		t.Fatalf("got %q want %q", q, expected)
	}
}
func TestStorageScopedToCredentialAndQuery(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	c, err := New("https://jira.example/jira", "one")
	if err != nil {
		t.Fatal(err)
	}
	cached := Cached{Query: "a", Issues: []Issue{{Key: "APP-1"}}, SyncedAt: time.Now()}
	if err := c.SaveCache(cached); err != nil {
		t.Fatal(err)
	}
	got, err := c.LoadCache("a")
	if err != nil || len(got.Issues) != 1 {
		t.Fatal(got, err)
	}
	if _, err := c.LoadCache("b"); err == nil {
		t.Fatal("query cache leaked")
	}
	other, _ := New("https://jira.example/jira", "two")
	if other.cachePath("a") == c.cachePath("a") {
		t.Fatal("credential cache leaked")
	}
	path := t.TempDir() + "/jira.json"
	config := Config{Queries: []Query{{Name: "Mine", JQL: "assignee = currentUser()"}}}
	if err := SaveConfig(path, config); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfig(path)
	if err != nil || len(loaded.Queries) != 1 {
		t.Fatal(loaded, err)
	}
}

func TestInvalidOrIncompleteResponseRetainsPreviousDataset(t *testing.T) {
	for _, body := range []string{`{}`, `{"startAt":0,"total":3,"issues":[]}`, `{"startAt":2,"total":3,"issues":[]}`, `{"startAt":0,"total":1,"issues":[{"key":"../admin"}]}`} {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) })
		if _, err := c.Search(context.Background(), "x"); err == nil {
			t.Fatalf("accepted incomplete response %s", body)
		}
	}
	calls := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Write([]byte(`{"startAt":0,"total":2,"issues":[{"key":"APP-1"}]}`))
			return
		}
		w.WriteHeader(500)
	})
	if rows, err := c.Search(context.Background(), "x"); err == nil || rows != nil {
		t.Fatal("later-page error returned replacement rows")
	}
}
func TestEmptyResultIsSuccessful(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"startAt":0,"total":0,"issues":[]}`)) })
	if rows, err := c.Search(context.Background(), "x"); err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
}

func TestLegacyJiraCacheMigration(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	c, err := New("https://jira.example", "token")
	if err != nil {
		t.Fatal(err)
	}
	path := c.cachePath("mine")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	original := `{"Query":"mine","Issues":[{"Key":"ABC-1"}],"SyncedAt":"2026-10-18T10:00:00Z"}`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := c.LoadCache("mine")
	if err != nil || len(data.Issues) != 1 || data.Issues[0].Key != "ABC-1" {
		t.Fatal(data, err)
	}
	if b, _ := os.ReadFile(path); string(b) != original {
		t.Fatal("legacy Jira cache modified")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	data, err = c.LoadCache("mine")
	if err != nil || data.SyncedAt.IsZero() || len(data.Issues) != 1 {
		t.Fatal(data, err)
	}
}
