package copilot

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const maxEventBytes = 2 << 20

type cursor struct {
	info     os.FileInfo
	position int64
	pending  []byte
	dropping bool
}

func obj(v any) map[string]any { m, _ := v.(map[string]any); return m }
func text(vs ...any) string {
	for _, v := range vs {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return ""
}
func printable(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
func number(v any) *float64 {
	var n float64
	switch v := v.(type) {
	case float64:
		n = v
	case json.Number:
		n, _ = v.Float64()
	case string:
		var err error
		n, err = strconv.ParseFloat(v, 64)
		if err != nil {
			return nil
		}
	default:
		return nil
	}
	if n < 0 || math.IsNaN(n) || math.IsInf(n, 0) {
		return nil
	}
	return &n
}
func tokens(v any) *int64 {
	if n := number(v); n != nil && *n < math.MaxInt64 {
		i := int64(*n)
		return &i
	}
	return nil
}
func timestamp(v any) string {
	if s, ok := v.(string); ok {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil && len(s) >= 16 {
			return time.Unix(0, n).UTC().Format("2006-01-02T15:04:05.000000000Z")
		}
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return t.UTC().Format("2006-01-02T15:04:05.000000000Z")
		}
	} else if n, ok := v.(float64); ok && n > 0 && n < 9e15 {
		return time.UnixMilli(int64(n)).UTC().Format("2006-01-02T15:04:05.000000000Z")
	}
	return ""
}
func metadata(m map[string]any) map[string]any {
	c := obj(m["context"])
	return map[string]any{"cwd": text(m["cwd"], m["working_directory"], c["cwd"]), "projectRoot": text(m["projectRoot"], m["project_root"], m["workspace_path"], m["workspacePath"]), "gitRoot": text(m["gitRoot"], m["git_root"], c["gitRoot"]), "branch": text(m["branch"], c["branch"]), "repository": text(m["repository"], c["repository"]), "remote": text(m["remote"], m["remote_url"]), "client": text(m["client_name"], m["clientName"]), "name": text(m["summary"], m["title"], m["name"])}
}
func localEvent(raw map[string]any, id string) Event {
	d := obj(raw["data"])
	kind := text(raw["type"], "unknown")
	b, _ := json.Marshal(raw)
	e := Event{Key: text(raw["id"], hash(raw)), EventID: text(raw["id"]), SessionID: id, Source: "local", Time: timestamp(raw["timestamp"]), Type: kind, Content: printable(d["content"]), CallID: text(d["toolCallId"]), Tool: text(d["toolName"]), Args: printable(d["arguments"]), Result: printable(d["result"]), Error: printable(d["error"]), Agent: text(raw["agentId"], d["agentName"]), Parent: text(d["parentToolCallId"]), Model: text(d["model"], d["modelId"], d["currentModel"]), TraceID: text(d["traceId"], d["trace_id"]), InputTokens: tokens(d["inputTokens"]), OutputTokens: tokens(d["outputTokens"]), DurationMs: number(d["durationMs"]), StartTime: timestamp(d["sessionStartTime"]), Meta: metadata(d), Raw: b}
	if kind == "user.message" {
		e.Role = "user"
	}
	if kind == "assistant.message" {
		e.Role = "assistant"
	}
	if kind == "assistant.usage" && e.DurationMs == nil {
		e.DurationMs = number(d["duration"])
	}
	switch kind {
	case "session.error", "error":
		e.State = "failed"
		e.Error = text(e.Error, d["message"])
	case "session.shutdown":
		e.State = "completed"
		if d["shutdownType"] == "error" {
			e.State = "failed"
		}
	case "session.task_complete":
		e.State = "completed"
	case "session.idle", "assistant.turn_end", "user_input.requested", "permission.requested":
		e.State = "waiting"
	case "user.message", "assistant.turn_start", "tool.execution_start":
		e.State = "active"
	case "session.warning":
		e.Warning = text(d["message"])
	}
	return e
}

// openRegular checks metadata before opening a file. A nil file with no error
// means the previously consumed file is unchanged. Changed files are checked
// again through the opened descriptor; symlinks are never imported.
func openRegular(path string, previous os.FileInfo) (*os.File, os.FileInfo, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, nil, errors.New("not a regular file")
	}
	if previous != nil && os.SameFile(previous, st) && previous.Size() == st.Size() && previous.ModTime() == st.ModTime() {
		return nil, st, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	current, err := f.Stat()
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(st, current) {
		f.Close()
		return nil, nil, errors.New("source file changed")
	}
	return f, current, nil
}
func (s *Service) report(message string) { s.mu.Lock(); s.addErrorLocked(message); s.mu.Unlock() }
func (s *Service) Scan(ctx context.Context) {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	if !s.options.Enabled || ctx.Err() != nil {
		return
	}
	if s.load() != nil {
		return
	}
	root, err := os.Lstat(s.options.SessionRoot)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil || !root.IsDir() || root.Mode()&os.ModeSymlink != 0 {
		s.report("Kan ikke lese sesjonsmappen som en vanlig mappe")
		return
	}
	dirs, err := os.ReadDir(s.options.SessionRoot)
	if err != nil {
		s.report("Kan ikke lese sesjonsmappen")
		return
	}
	count := 0
	for _, dir := range dirs {
		if ctx.Err() != nil {
			return
		}
		if !dir.IsDir() {
			continue
		}
		count++
		id := dir.Name()
		path := filepath.Join(s.options.SessionRoot, id)
		s.importWorkspace(ctx, id, filepath.Join(path, "workspace.yaml"))
		s.importEvents(ctx, id, filepath.Join(path, "events.jsonl"))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.diagnostics.Discovered = count
	if err := s.saveLocked(); err != nil {
		s.reportSaveErrorLocked("Copilot", err)
	}
}
func (s *Service) importWorkspace(ctx context.Context, id, path string) {
	old := s.cursors[path]
	var previous os.FileInfo
	if old != nil {
		previous = old.info
	}
	f, info, err := openRegular(path, previous)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		s.report(id + ": workspace.yaml er ikke lesbar eller er en symlink")
		return
	}
	if f == nil {
		return
	}
	defer f.Close()
	if old != nil && os.SameFile(old.info, info) && old.info.ModTime() == info.ModTime() && old.info.Size() == info.Size() {
		return
	}
	if info.Size() > 1<<20 {
		s.cursors[path] = &cursor{info: info}
		s.report(id + ": workspace.yaml overskrider 1 MiB")
		return
	}
	b, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		s.report(id + ": kunne ikke lese workspace.yaml")
		return
	}
	// A fully read but malformed workspace should only be parsed again after
	// it changes. Read failures above remain retryable without a metadata change.
	s.cursors[path] = &cursor{info: info}
	var m map[string]any
	if yaml.Unmarshal(b, &m) != nil || m == nil {
		s.report(id + ": ugyldig workspace.yaml")
		return
	}
	meta := metadata(m)
	enrichProject(ctx, meta)
	s.mu.Lock()
	s.metadataLocked(id, meta)
	s.mu.Unlock()
}
func (s *Service) acceptLine(ctx context.Context, id string, b []byte, complete bool) {
	if len(bytes.TrimSpace(b)) == 0 {
		return
	}
	var raw map[string]any
	if json.Unmarshal(b, &raw) != nil || len(raw) == 0 {
		if complete {
			s.report(id + ": ugyldig JSONL-hendelse")
		}
		return
	}
	e := localEvent(raw, id)
	if e.Type == "session.start" || e.Type == "session.context_changed" {
		enrichProject(ctx, e.Meta)
	}
	s.mu.Lock()
	s.ingestLocked([]Event{e})
	s.mu.Unlock()
}
func (s *Service) importEvents(ctx context.Context, id, path string) {
	c := s.cursors[path]
	var previous os.FileInfo
	// A cancelled or failed read may leave bytes unread even when metadata has
	// not changed. Only fully consumed snapshots can skip opening the file.
	if c != nil && c.position == c.info.Size() {
		previous = c.info
	}
	f, info, err := openRegular(path, previous)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		s.report(id + ": events.jsonl er ikke lesbar eller er en symlink")
		return
	}
	if f == nil {
		return
	}
	defer f.Close()
	if c == nil || !os.SameFile(c.info, info) || info.Size() < c.position || (info.Size() == c.position && info.ModTime() != c.info.ModTime()) {
		c = &cursor{info: info}
		s.cursors[path] = c
	}
	if c.position == info.Size() && c.info.ModTime() == info.ModTime() {
		return
	}
	reader := bufio.NewReaderSize(io.NewSectionReader(f, c.position, info.Size()-c.position), 64<<10)
	for ctx.Err() == nil {
		chunk, err := reader.ReadSlice('\n')
		c.position += int64(len(chunk))
		if !c.dropping {
			if len(c.pending)+len(chunk) > maxEventBytes {
				c.pending = nil
				c.dropping = true
				s.report(id + ": hendelse overskrider 2 MiB")
			} else {
				c.pending = append(c.pending, chunk...)
			}
		}
		if len(chunk) > 0 && chunk[len(chunk)-1] == '\n' {
			if !c.dropping {
				s.acceptLine(ctx, id, c.pending, true)
			}
			c.pending = nil
			c.dropping = false
		}
		if err == io.EOF {
			break
		}
		if err != nil && err != bufio.ErrBufferFull {
			s.report(id + ": kunne ikke lese events.jsonl")
			break
		}
	}
	if !c.dropping && len(c.pending) > 0 {
		s.acceptLine(ctx, id, c.pending, false)
	}
	c.info = info
}

type boundedOutput struct{ bytes.Buffer }

func (b *boundedOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 32768 {
		return 0, errors.New("git output too large")
	}
	return b.Buffer.Write(p)
}
func enrichProject(ctx context.Context, meta map[string]any) {
	cwd := text(meta["cwd"])
	if !filepath.IsAbs(cwd) {
		return
	}
	git := func(args ...string) string {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", cwd}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
		output := &boundedOutput{}
		cmd.Stdout = output
		if cmd.Run() != nil {
			return ""
		}
		return strings.TrimSpace(output.String())
	}
	root := text(git("rev-parse", "--show-toplevel"), meta["gitRoot"])
	meta["gitRoot"] = root
	if explicit := text(meta["projectRoot"]); !filepath.IsAbs(explicit) {
		meta["projectRoot"] = root
	}
	if text(meta["branch"]) == "" {
		meta["branch"] = git("symbolic-ref", "--short", "HEAD")
	}
	if text(meta["remote"]) == "" {
		meta["remote"] = git("config", "--get", "remote.origin.url")
	}
}
