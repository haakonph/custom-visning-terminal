package copilot

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/haakonph/custom-visning-terminal/internal/storage"
)

// Audit actual SQL writes, including cascades, so a snapshot replacement cannot
// pass simply because it produces the same final data.
func auditCopilotWrites(t *testing.T, o Options) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", storage.Path(o.StorePath))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE write_audit (kind TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"copilot_sessions", "copilot_events", "copilot_usage", "copilot_traces"} {
		for _, action := range []string{"INSERT", "UPDATE", "DELETE"} {
			statement := fmt.Sprintf("CREATE TRIGGER audit_%s_%s AFTER %s ON %s BEGIN INSERT INTO write_audit VALUES ('%s:%s'); END", table, action, action, table, table, action)
			if _, err := db.Exec(statement); err != nil {
				t.Fatal(err)
			}
		}
	}
	return db
}

func expectCopilotWrites(t *testing.T, db *sql.DB, want map[string]int) {
	t.Helper()
	rows, err := db.Query("SELECT kind, COUNT(*) FROM write_audit GROUP BY kind")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for rows.Next() {
		var kind string
		var count int
		if err := rows.Scan(&kind, &count); err != nil {
			t.Fatal(err)
		}
		got[kind] = count
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if len(got) != 0 || len(want) != 0 {
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("SQL writes: got %v, want %v", got, want)
		}
	}
	if _, err := db.Exec("DELETE FROM write_audit"); err != nil {
		t.Fatal(err)
	}
}

func saveCopilot(t *testing.T, s *Service) {
	t.Helper()
	if err := s.saveLocked(); err != nil {
		t.Fatal(err)
	}
}

func TestIncrementalCopilotWritesOnlyChangedRows(t *testing.T) {
	o := testOptions(t)
	s := service(t, o)
	input := int64(10)
	initial := []Event{
		{Source: "local", SessionID: "s1", Key: "usage", Type: "assistant.usage", InputTokens: &input},
		{Source: "local", SessionID: "s1", Key: "history", Content: "keep this history"},
		{Source: "otel", SessionID: "s2", Key: "other", TraceID: "other-trace", Content: "untouched session"},
	}
	s.ingestLocked(initial)
	saveCopilot(t, s)
	db := auditCopilotWrites(t, o)

	next := Event{Source: "local", SessionID: "s1", Key: "new", Type: "assistant.usage", InputTokens: &input, TraceID: "new-trace", Time: "2026-10-17T10:00:00Z"}
	s.ingestLocked([]Event{next})
	saveCopilot(t, s)
	expectCopilotWrites(t, db, map[string]int{"copilot_sessions:UPDATE": 1, "copilot_events:INSERT": 1, "copilot_usage:INSERT": 1, "copilot_traces:INSERT": 1})

	// Exact duplicates and an idle flush must not touch any stored rows.
	s.ingestLocked([]Event{next})
	saveCopilot(t, s)
	saveCopilot(t, s)
	expectCopilotWrites(t, db, nil)

	// Replacing an event must also remove usage that it no longer supplies.
	next.InputTokens = nil
	next.Content = "updated without usage"
	s.ingestLocked([]Event{next})
	saveCopilot(t, s)
	expectCopilotWrites(t, db, map[string]int{"copilot_sessions:UPDATE": 1, "copilot_events:UPDATE": 1, "copilot_usage:DELETE": 1})
	stale := next
	stale.Time = "2026-10-16T10:00:00Z"
	stale.Content = "stale replacement"
	s.ingestLocked([]Event{stale})
	saveCopilot(t, s)
	expectCopilotWrites(t, db, nil)

	s.metadataLocked("s1", map[string]any{"name": "new summary"})
	saveCopilot(t, s)
	expectCopilotWrites(t, db, map[string]int{"copilot_sessions:UPDATE": 1})

	// Reopening and replaying the original input preserves deduplication.
	reopened := service(t, o)
	reopened.ingestLocked(append(initial, next))
	saveCopilot(t, reopened)
	expectCopilotWrites(t, db, nil)
	detail, err := reopened.Detail(context.Background(), "s1")
	if err != nil || len(detail.Events) != 3 || detail.Session.Name != "new summary" || detail.Session.InputTokens == nil || *detail.Session.InputTokens != 10 {
		t.Fatalf("incremental history/usage: %+v, %v", detail, err)
	}
}

func TestIncrementalCopilotSaveFailureRollsBackAndRetries(t *testing.T) {
	o := testOptions(t)
	s := service(t, o)
	s.ingestLocked([]Event{{Source: "local", SessionID: "s1", Key: "saved"}})
	saveCopilot(t, s)
	db := auditCopilotWrites(t, o)
	if _, err := db.Exec(`CREATE TRIGGER fail_events BEFORE INSERT ON copilot_events BEGIN SELECT RAISE(ABORT, 'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	s.metadataLocked("s1", map[string]any{"name": "retry summary"})
	s.ingestLocked([]Event{{Source: "otel", SessionID: "s1", Key: "retry", TraceID: "retry-trace"}})
	if err := s.saveLocked(); err == nil {
		t.Fatal("expected injected failure")
	}
	expectCopilotWrites(t, db, nil)
	before := service(t, o)
	detail, err := before.Detail(context.Background(), "s1")
	if err != nil || len(detail.Events) != 1 || detail.Session.Name == "retry summary" {
		t.Fatalf("partial transaction committed: %+v %v", detail, err)
	}
	if _, err := db.Exec("DROP TRIGGER fail_events"); err != nil {
		t.Fatal(err)
	}
	saveCopilot(t, s)
	expectCopilotWrites(t, db, map[string]int{"copilot_sessions:UPDATE": 1, "copilot_events:INSERT": 1, "copilot_traces:INSERT": 1})
	after := service(t, o)
	detail, err = after.Detail(context.Background(), "s1")
	if err != nil || len(detail.Events) != 2 || detail.Session.Name != "retry summary" || after.data.Traces["retry-trace"] != "s1" {
		t.Fatalf("retry lost pending data: %+v %v", detail, err)
	}
}

func TestIncrementalCopilotTraceSessionMerge(t *testing.T) {
	for _, flushTemporary := range []bool{false, true} {
		t.Run(fmt.Sprintf("persisted-temporary-%t", flushTemporary), func(t *testing.T) {
			o := testOptions(t)
			s := service(t, o)
			input := int64(7)
			s.ingestLocked([]Event{
				{Source: "otel", Key: "span:t:p", TraceID: "t", SpanID: "p", InputTokens: &input},
				{Source: "otel", Key: "log:other", SessionID: "trace:t", TraceID: "other"},
			})
			s.metadataLocked("trace:t", map[string]any{"cwd": "/work/preserved"})
			if flushTemporary {
				saveCopilot(t, s)
				s = service(t, o)
			}
			s.ingestLocked([]Event{{Source: "otel", Key: "log:correlate", SessionID: "real", TraceID: "t"}})
			if flushTemporary {
				db := auditCopilotWrites(t, o)
				if _, err := db.Exec(`CREATE TRIGGER fail_trace BEFORE INSERT ON copilot_traces BEGIN SELECT RAISE(ABORT, 'injected failure'); END`); err != nil {
					t.Fatal(err)
				}
				if err := s.saveLocked(); err == nil {
					t.Fatal("expected trace failure during merge")
				}
				expectCopilotWrites(t, db, nil)
				unchanged := service(t, o)
				if unchanged.data.Sessions["trace:t"] == nil || unchanged.data.Sessions["real"] != nil || unchanged.data.Traces["t"] != "trace:t" {
					t.Fatal("failed merge committed partial deletion or trace mapping")
				}
				if _, err := db.Exec("DROP TRIGGER fail_trace"); err != nil {
					t.Fatal(err)
				}
			}
			saveCopilot(t, s)
			s = service(t, o)
			detail, err := s.Detail(context.Background(), "real")
			if err != nil || len(s.data.Sessions) != 1 || len(detail.Events) != 3 || detail.Session.Cwd != "/work/preserved" || detail.Session.InputTokens == nil || *detail.Session.InputTokens != 7 {
				t.Fatalf("trace merge lost data: %+v %v", detail, err)
			}
			for _, trace := range []string{"t", "other"} {
				if s.data.Traces[trace] != "real" {
					t.Fatalf("trace %s points to removed session: %v", trace, s.data.Traces)
				}
			}
		})
	}
}

func TestIncrementalCopilotPreservesOtherWritersEvents(t *testing.T) {
	o := testOptions(t)
	first := service(t, o)
	first.ingestLocked([]Event{{Source: "local", SessionID: "s1", Key: "original"}})
	saveCopilot(t, first)
	second := service(t, o)
	first.ingestLocked([]Event{{Source: "local", SessionID: "s1", Key: "first"}})
	second.ingestLocked([]Event{
		{Source: "local", SessionID: "s1", Key: "second"},
		{Source: "local", SessionID: "s2", Key: "separate"},
	})
	saveCopilot(t, second)
	saveCopilot(t, first)
	reopened := service(t, o)
	detail, err := reopened.Detail(context.Background(), "s1")
	if err != nil || len(detail.Events) != 3 || len(reopened.data.Sessions) != 2 {
		t.Fatalf("stale snapshot overwrote another writer: %+v %v", detail, err)
	}
}

func TestIncrementalCopilotAfterLegacyMigration(t *testing.T) {
	o := testOptions(t)
	original := `{"version":1,"sessions":{"s1":{"metadata":{"cwd":"/work/repo"},"events":{"local:old":{"source":"local","key":"old","sessionId":"s1"}}}},"traces":{}}`
	write(t, o.StorePath, original)
	s := service(t, o)
	db := auditCopilotWrites(t, o)
	s.ingestLocked([]Event{{Source: "local", SessionID: "s1", Key: "new"}})
	saveCopilot(t, s)
	expectCopilotWrites(t, db, map[string]int{"copilot_sessions:UPDATE": 1, "copilot_events:INSERT": 1})
	reopened := service(t, o)
	detail, err := reopened.Detail(context.Background(), "s1")
	if err != nil || len(detail.Events) != 2 || detail.Session.Cwd != "/work/repo" {
		t.Fatalf("legacy history lost: %+v %v", detail, err)
	}
}

func TestCopilotSummaryCacheInvalidationAndAging(t *testing.T) {
	s := service(t, testOptions(t))
	s.ingestLocked([]Event{{Source: "local", SessionID: "s1", Key: "start", Time: time.Now().UTC().Format(time.RFC3339Nano), State: "active"}})
	rows, err := s.Sessions(context.Background())
	if err != nil || len(rows) != 1 || rows[0].Status != "active" {
		t.Fatal(rows, err)
	}
	// Advance the cached activity time without changing the underlying events;
	// repeated readers should reuse the cache and still age inferred activity.
	cached := s.summaries["s1"]
	cached.LastActivity = time.Now().Add(-3 * time.Minute).UTC().Format(time.RFC3339Nano)
	s.summaries["s1"] = cached
	rows, err = s.Sessions(context.Background())
	if err != nil || rows[0].Status != "inactive" {
		t.Fatal(rows, err)
	}
	s.metadataLocked("s1", map[string]any{"name": "changed name"})
	rows, err = s.Sessions(context.Background())
	if err != nil || rows[0].Name != "changed name" || rows[0].Status != "active" {
		t.Fatal(rows, err)
	}
	s.ingestLocked([]Event{{Source: "local", SessionID: "s1", Key: "end", Time: time.Now().Add(time.Second).UTC().Format(time.RFC3339Nano), State: "completed"}})
	rows, err = s.Sessions(context.Background())
	if err != nil || rows[0].Status != "completed" || rows[0].StatusSource != "confirmed" {
		t.Fatal(rows, err)
	}
}

func BenchmarkIncrementalCopilot(b *testing.B) {
	for _, count := range []int{1000, 10000} {
		b.Run(fmt.Sprintf("events-%d", count), func(b *testing.B) {
			dir := b.TempDir()
			s, err := New(Options{SessionRoot: filepath.Join(dir, "sessions"), StorePath: filepath.Join(dir, "store.sqlite"), CaptureContent: true, ShowContent: true, JiraPattern: `[A-Z]+-[0-9]+`})
			if err != nil {
				b.Fatal(err)
			}
			for i := range count {
				s.ingestLocked([]Event{{SessionID: fmt.Sprintf("session-%d", i/100), Source: "local", Key: fmt.Sprint(i), Content: "Investigate APP-42"}})
			}
			if err := s.saveLocked(); err != nil {
				b.Fatal(err)
			}
			b.Run("save-one-change", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; b.Loop(); i++ {
					s.ingestLocked([]Event{{SessionID: "session-0", Source: "local", Key: "0", Content: fmt.Sprintf("update %d", i)}})
					if err := s.saveLocked(); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("unchanged-session-list", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := s.Sessions(context.Background()); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}
