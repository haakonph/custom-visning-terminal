package copilot

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haakonph/custom-visning-terminal/internal/storage"
)

func testOptions(t *testing.T) Options {
	t.Helper()
	dir := t.TempDir()
	return Options{SessionRoot: filepath.Join(dir, "sessions"), StorePath: filepath.Join(dir, "data", "copilot.json"), Enabled: true, CaptureContent: true, ShowContent: true, Port: 0, JiraPattern: `[A-Z][A-Z0-9_]*-[1-9][0-9]*`}
}
func service(t *testing.T, o Options) *Service {
	t.Helper()
	s, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func eventJSON(id, kind string, data map[string]any) string {
	b, _ := json.Marshal(map[string]any{"id": id, "type": kind, "timestamp": "2026-10-17T10:00:00Z", "data": data})
	return string(b) + "\n"
}
func TestIndependentLocalCollectionPersistenceAndUsage(t *testing.T) {
	ctx := context.Background()
	o := testOptions(t)
	dir := filepath.Join(o.SessionRoot, "s1")
	write(t, filepath.Join(dir, "workspace.yaml"), "summary: Login fix\nclient_name: copilot-intellij\nproject_root: /work/api\ncontext:\n  cwd: /work/api\n  branch: feat/TEST-42-login\n  repository: acme/api\n")
	input := eventJSON("1", "user.message", map[string]any{"content": "Fix login"}) + eventJSON("2", "assistant.message", map[string]any{"content": "Done"}) + eventJSON("3", "tool.execution_start", map[string]any{"toolCallId": "call1", "toolName": "shell", "arguments": map[string]any{"command": "go test"}}) + eventJSON("4", "tool.execution_complete", map[string]any{"toolCallId": "call1", "toolName": "shell", "result": "passed"}) + eventJSON("5", "assistant.usage", map[string]any{"inputTokens": 10, "outputTokens": 4, "model": "gpt-test", "durationMs": 150}) + eventJSON("6", "session.task_complete", map[string]any{})
	write(t, filepath.Join(dir, "events.jsonl"), input)
	s := service(t, o)
	s.Scan(ctx)
	s.Scan(ctx)
	rows, err := s.Sessions(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatalf("sessions: %+v %v", rows, err)
	}
	row := rows[0]
	if row.Name != "Login fix" || row.Client != "copilot-intellij" || row.Jira != "TEST-42" || row.ProjectRoot != "/work/api" || row.Status != "completed" || row.StatusSource != "confirmed" || row.Model != "gpt-test" || row.InputTokens == nil || *row.InputTokens != 10 {
		t.Fatalf("incorrect local summary: %+v", row)
	}
	detail, err := s.Detail(ctx, "s1")
	if err != nil || len(detail.Events) != 6 {
		t.Fatalf("events duplicated or absent: %+v %v", detail, err)
	}
	if detail.Events[3].Result != "passed" {
		t.Fatalf("tool result: %+v", detail.Events)
	}
	info, err := os.Stat(storage.Path(o.StorePath))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("private store: %v %v", info, err)
	}
	// The source files and either other app are unnecessary to read persisted history.
	if err := os.RemoveAll(o.SessionRoot); err != nil {
		t.Fatal(err)
	}
	restarted := service(t, o)
	restarted.Scan(ctx)
	rows, _ = restarted.Sessions(ctx)
	if len(rows) != 1 || *rows[0].InputTokens != 10 {
		t.Fatalf("history lost: %+v", rows)
	}
}
func TestPartialLinesRotationAndMalformedEvents(t *testing.T) {
	ctx := context.Background()
	o := testOptions(t)
	file := filepath.Join(o.SessionRoot, "s1", "events.jsonl")
	line := eventJSON("a", "user.message", map[string]any{"content": "æøå"})
	split := strings.Index(line, "æ") + 1
	write(t, file, line[:split])
	s := service(t, o)
	s.Scan(ctx)
	rows, _ := s.Sessions(ctx)
	if len(rows) != 0 {
		t.Fatalf("imported partial JSON: %+v", rows)
	}
	f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString(line[split:] + "invalid\n" + strings.Repeat("x", maxEventBytes+20) + "\n" + eventJSON("b", "assistant.message", map[string]any{"content": "hello"}))
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	s.Scan(ctx)
	d, _ := s.Detail(ctx, "s1")
	if len(d.Events) != 2 || d.Events[0].Content != "æøå" {
		t.Fatalf("incremental UTF-8: %+v", d.Events)
	}
	diag, _ := s.Diagnostics(ctx)
	if len(diag.Errors) != 2 {
		t.Fatalf("missing bounded diagnostics: %+v", diag)
	}
	if err := os.Rename(file, file+".old"); err != nil {
		t.Fatal(err)
	}
	write(t, file, strings.TrimSuffix(eventJSON("c", "assistant.message", map[string]any{"content": "rotated"}), "\n"))
	s.Scan(ctx)
	s.Scan(ctx)
	d, _ = s.Detail(ctx, "s1")
	if len(d.Events) != 3 {
		t.Fatalf("rotation/tail duplicated or lost: %+v", d.Events)
	}
	write(t, file, eventJSON("d", "session.idle", map[string]any{}))
	s.Scan(ctx)
	d, _ = s.Detail(ctx, "s1")
	if len(d.Events) != 4 {
		t.Fatalf("truncation not read: %+v", d.Events)
	}
}
func TestSymlinksAndInvalidWorkspaceAreNotImported(t *testing.T) {
	ctx := context.Background()
	o := testOptions(t)
	outside := t.TempDir()
	write(t, filepath.Join(outside, "events.jsonl"), eventJSON("x", "user.message", map[string]any{"content": "outside-secret"}))
	if err := os.MkdirAll(o.SessionRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(o.SessionRoot, "linked-dir")); err != nil {
		t.Skip(err)
	}
	dir := filepath.Join(o.SessionRoot, "s1")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "events.jsonl"), filepath.Join(dir, "events.jsonl")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "workspace.yaml"), "summary: first\nsummary: duplicate\n")
	s := service(t, o)
	s.Scan(ctx)
	rows, _ := s.Sessions(ctx)
	if len(rows) != 0 {
		t.Fatal("imported symlink or invalid workspace")
	}
	diag, _ := s.Diagnostics(ctx)
	if len(diag.Errors) != 2 {
		t.Fatalf("diagnostics: %+v", diag)
	}
}
func TestContentControlsAndDisabledCollection(t *testing.T) {
	ctx := context.Background()
	o := testOptions(t)
	file := filepath.Join(o.SessionRoot, "s1", "events.jsonl")
	write(t, file, eventJSON("1", "user.message", map[string]any{"content": "secret-prompt", "title": "secret-title", "context": map[string]any{"repository": "acme/api"}})+eventJSON("2", "tool.execution_complete", map[string]any{"arguments": "secret-args", "result": "secret-result", "error": "secret-error"}))
	o.CaptureContent = false
	s := service(t, o)
	s.Scan(ctx)
	content, err := os.ReadFile(storage.Path(o.StorePath))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "secret-") {
		t.Fatalf("content captured despite opt-out: %s", content)
	}
	// ShowContent also masks content previously stored with capture enabled.
	o.CaptureContent = true
	s = service(t, o)
	s.Scan(ctx)
	o.ShowContent = false
	s = service(t, o)
	rows, _ := s.Sessions(ctx)
	d, _ := s.Detail(ctx, "s1")
	encoded, _ := json.Marshal(d)
	if rows[0].Name != "s1" || strings.Contains(string(encoded), "secret-") {
		t.Fatalf("stored content exposed: %s", encoded)
	}
	o.Enabled = false
	s = service(t, o)
	write(t, filepath.Join(o.SessionRoot, "new", "events.jsonl"), eventJSON("new", "user.message", map[string]any{"content": "new-secret"}))
	s.Scan(ctx)
	rows, _ = s.Sessions(ctx)
	if len(rows) != 1 {
		t.Fatal("disabled collection imported data")
	}
}
func TestTraceCorrelationDedupAndUsagePreference(t *testing.T) {
	ctx := context.Background()
	s := service(t, testOptions(t))
	input := int64(99)
	local := int64(12)
	s.mu.Lock()
	s.ingestLocked([]Event{{Source: "otel", Key: "span:trace:span", TraceID: "trace", SpanID: "span", InputTokens: &input, Time: "2026-10-17T10:00:00.000000000Z"}, {Source: "otel", Key: "log:1", TraceID: "trace", SessionID: "s1", Type: "log", Time: "2026-10-17T10:00:01.000000000Z"}, {Source: "local", SessionID: "s1", Key: "usage", Type: "assistant.usage", InputTokens: &local}})
	s.ingestLocked([]Event{{Source: "local", SessionID: "s1", Key: "usage", Type: "assistant.usage", InputTokens: &local}})
	s.mu.Unlock()
	rows, _ := s.Sessions(ctx)
	if len(rows) != 1 || rows[0].ID != "s1" || *rows[0].InputTokens != 12 {
		t.Fatalf("correlation/usage: %+v", rows)
	}
	d, _ := s.Detail(ctx, "s1")
	if len(d.Events) != 3 {
		t.Fatalf("dedup: %+v", d.Events)
	}
	parent, child := int64(100), int64(7)
	chosen := preferredUsage([]Event{{Source: "otel", Key: "span:t:p", TraceID: "t", SpanID: "p", InputTokens: &parent}, {Source: "otel", Key: "span:t:c", TraceID: "t", SpanID: "c", Parent: "p", InputTokens: &child}})
	if len(chosen) != 1 || *chosen[0].InputTokens != 7 {
		t.Fatal("nested spans double-counted")
	}
	chosen = preferredUsage([]Event{{Key: "metric:1", MetricSeries: "input", Temporality: 2, InputTokens: &child}, {Key: "metric:2", MetricSeries: "input", Temporality: 2, InputTokens: &parent}})
	if len(chosen) != 1 || *chosen[0].InputTokens != 100 {
		t.Fatal("cumulative metrics added together")
	}
}
func TestStoreCorruptionAndConfiguration(t *testing.T) {
	o := testOptions(t)
	write(t, o.StorePath, "invalid private data")
	if _, err := New(o); err == nil || strings.Contains(err.Error(), "private data") {
		t.Fatalf("corrupt store: %v", err)
	}
	b, _ := os.ReadFile(o.StorePath)
	if string(b) != "invalid private data" {
		t.Fatal("overwrote corrupt store")
	}
	t.Setenv("COPILOT_OTLP_PORT", "bad")
	if _, err := OptionsFromEnv(); err == nil {
		t.Fatal("accepted invalid port")
	}
	t.Setenv("COPILOT_OTLP_PORT", "4319")
	t.Setenv("COPILOT_COLLECTION_ENABLED", "false")
	configured, err := OptionsFromEnv()
	if err != nil || configured.Enabled || configured.Port != 4319 {
		t.Fatalf("env config: %+v %v", configured, err)
	}
	t.Setenv("COPILOT_SHOW_CONTENT", "invalid")
	if _, err := OptionsFromEnv(); err == nil {
		t.Fatal("accepted invalid boolean")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := service(t, testOptions(t))
	if _, err := s.Sessions(ctx); err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestLegacyCopilotMigration(t *testing.T) {
	o := testOptions(t)
	original := `{"version":1,"sessions":{"existing":{"metadata":{"cwd":"/work/repo"},"events":{}}},"traces":{"trace-id":"existing"}}`
	write(t, o.StorePath, original)
	s := service(t, o)
	rows, err := s.Sessions(context.Background())
	if err != nil || len(rows) != 1 || rows[0].Cwd != "/work/repo" {
		t.Fatal(rows, err)
	}
	if b, _ := os.ReadFile(o.StorePath); string(b) != original {
		t.Fatal("legacy Copilot store changed")
	}
	if err := os.Remove(o.StorePath); err != nil {
		t.Fatal(err)
	}
	s = service(t, o)
	if s.data.Traces["trace-id"] != "existing" {
		t.Fatal("trace mapping lost")
	}
	rows, err = s.Sessions(context.Background())
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
}

func TestStoreLargerThanFormerReadLimit(t *testing.T) {
	o := testOptions(t)
	write(t, o.StorePath, "")
	f, err := os.OpenFile(o.StorePath, os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	// Valid JSON with enough whitespace inside the object to cross the old
	// limit, without constructing a huge session fixture in memory.
	if _, err := io.WriteString(f, `{"version":1,"sessions":{},"traces":{},`); err != nil {
		t.Fatal(err)
	}
	padding := strings.Repeat(" ", 1<<20)
	for range 128 {
		if _, err := io.WriteString(f, padding); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := io.WriteString(f, `"extra":null}`); err != nil {
		t.Fatal(err)
	}
	s, err := New(o)
	if err != nil {
		t.Fatalf("valid large store could not be reopened: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	// SQLite now takes precedence after import. Remove the migrated database
	// to exercise validation of the large legacy input again.
	if err := os.Remove(storage.Path(o.StorePath)); err != nil {
		t.Fatal(err)
	}
	// Invalid trailing content must not be hidden beyond the former limit.
	if _, err := io.WriteString(f, " private-invalid-tail"); err != nil {
		t.Fatal(err)
	}
	if _, err := New(o); err == nil || strings.Contains(err.Error(), "private-invalid-tail") {
		t.Fatalf("invalid large store: %v", err)
	}
}

func TestInvalidStoresPreservedWithRecoveryGuidance(t *testing.T) {
	for _, content := range []string{
		"", `{"version":1`, `{"version":2,"sessions":{},"traces":{}}`,
		`{"version":1,"sessions":{"s":null},"traces":{}}`,
		`{"version":1,"sessions":{},"traces":{}} private-invalid-tail`,
	} {
		t.Run(content, func(t *testing.T) {
			o := testOptions(t)
			write(t, o.StorePath, content)
			_, err := New(o)
			if err == nil {
				t.Fatal("accepted invalid store")
			}
			for _, want := range []string{o.StorePath, "existing file has been preserved", "COPILOT_STORE_PATH"} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("missing recovery guidance %q: %v", want, err)
				}
			}
			if strings.Contains(err.Error(), "private-invalid-tail") {
				t.Fatalf("file contents leaked: %v", err)
			}
			got, readErr := os.ReadFile(o.StorePath)
			if readErr != nil || string(got) != content {
				t.Fatalf("original store changed: %v", readErr)
			}
		})
	}
}
