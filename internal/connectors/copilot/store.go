package copilot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/storage"
)

type storedSession = model.CopilotStoredSession
type diskStore = model.CopilotStore

func hash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (s *Service) sessionLocked(id string) *storedSession {
	v := s.data.Sessions[id]
	if v == nil {
		v = &storedSession{Meta: map[string]any{}, Events: map[string]Event{}}
		s.data.Sessions[id] = v
		s.markSessionLocked(id)
	}
	return v
}
func (s *Service) metadataLocked(id string, m map[string]any) {
	v := s.sessionLocked(id)
	for k, val := range m {
		if printable(val) == "" {
			continue
		}
		if !s.options.CaptureContent && !safeMetadata[k] {
			continue
		}
		if hash(v.Meta[k]) != hash(val) {
			v.Meta[k] = val
			s.markSessionLocked(id)
		}
	}
}

var safeMetadata = map[string]bool{"cwd": true, "projectRoot": true, "gitRoot": true, "branch": true, "repository": true, "remote": true, "client": true}

func withoutContent(e Event) Event {
	e.Content = ""
	e.Args = ""
	e.Result = ""
	e.Raw = nil
	if e.Error != "" {
		e.Error = "Error (content disabled)"
	}
	if e.Warning != "" {
		e.Warning = "Warning (content disabled)"
	}
	meta := map[string]any{}
	for k, v := range e.Meta {
		if safeMetadata[k] {
			meta[k] = v
		}
	}
	e.Meta = meta
	return e
}
func (s *Service) ingestLocked(events []Event) {
	for _, e := range events {
		if !s.options.CaptureContent {
			e = withoutContent(e)
		}
		id := e.SessionID
		if e.TraceID != "" {
			previous := s.data.Traces[e.TraceID]
			if id == "" {
				id = previous
			}
			if id == "" {
				id = "trace:" + e.TraceID
			}
			if previous != "" && previous != id && previous == "trace:"+e.TraceID {
				if old := s.data.Sessions[previous]; old != nil {
					v := s.sessionLocked(id)
					for k, ev := range old.Events {
						v.Events[k] = ev
						s.markEventLocked(id, k)
					}
					for k, val := range old.Meta {
						if _, ok := v.Meta[k]; !ok {
							v.Meta[k] = val
						}
					}
					delete(s.data.Sessions, previous)
					delete(s.changedSessions, previous)
					delete(s.summaries, previous)
					if s.removedSessions == nil {
						s.removedSessions = map[string]bool{}
					}
					s.removedSessions[previous] = true
					s.markSessionLocked(id)
					for trace, session := range s.data.Traces {
						if session == previous {
							s.setTraceLocked(trace, id)
						}
					}
				}
			}
			if s.data.Traces[e.TraceID] != id {
				s.setTraceLocked(e.TraceID, id)
			}
		}
		if id == "" {
			id = "unattributed"
		}
		e.SessionID = id
		v := s.sessionLocked(id)
		key := e.Source + ":" + e.Key
		if old, ok := v.Events[key]; ok {
			if old.Time > e.Time || hash(old) == hash(e) {
				continue
			}
		}
		v.Events[key] = e
		s.markEventLocked(id, key)
	}
}

func (s *Service) markSessionLocked(id string) {
	if s.changedSessions == nil {
		s.changedSessions = map[string]map[string]bool{}
	}
	if s.changedSessions[id] == nil {
		s.changedSessions[id] = map[string]bool{}
	}
	delete(s.summaries, id)
}

func (s *Service) markEventLocked(id, key string) {
	s.markSessionLocked(id)
	s.changedSessions[id][key] = true
}

func (s *Service) setTraceLocked(trace, id string) {
	s.data.Traces[trace] = id
	if s.changedTraces == nil {
		s.changedTraces = map[string]bool{}
	}
	s.changedTraces[trace] = true
}

func (s *Service) saveLocked() error {
	if len(s.changedSessions) == 0 && len(s.changedTraces) == 0 && len(s.removedSessions) == 0 {
		return nil
	}
	changes := diskStore{Version: 1, Sessions: map[string]*storedSession{}, Traces: map[string]string{}, Summaries: map[string]Session{}}
	for id, keys := range s.changedSessions {
		session := s.data.Sessions[id]
		events := make(map[string]Event, len(keys))
		for key := range keys {
			events[key] = session.Events[key]
		}
		changes.Sessions[id] = &storedSession{Meta: session.Meta, Events: events}
		changes.Summaries[id] = s.summaryLocked(id, session)
	}
	for trace := range s.changedTraces {
		changes.Traces[trace] = s.data.Traces[trace]
	}
	removed := make([]string, 0, len(s.removedSessions))
	for id := range s.removedSessions {
		removed = append(removed, id)
	}
	if err := storage.SaveCopilotChanges(s.options.StorePath, changes, removed); err != nil {
		return err
	}
	s.changedSessions, s.changedTraces, s.removedSessions = nil, nil, nil
	return nil
}
func sortedEvents(v *storedSession) []Event {
	events := make([]Event, 0, len(v.Events))
	for _, e := range v.Events {
		events = append(events, e)
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].Time != events[j].Time {
			return events[i].Time < events[j].Time
		}
		return events[i].Key < events[j].Key
	})
	return events
}
func (s *Service) summaryLocked(id string, v *storedSession) Session {
	if s.summaries == nil {
		s.summaries = map[string]Session{}
	}
	out, ok := s.summaries[id]
	if !ok {
		out = s.buildSummaryLocked(id, v)
		s.summaries[id] = out
	}
	// Inferred activity ages even when the underlying events do not change.
	if out.StatusSource == "inferred" {
		if at, err := time.Parse(time.RFC3339Nano, out.LastActivity); err == nil {
			out.Status = "inactive"
			if time.Since(at) < 2*time.Minute {
				out.Status = "active"
			}
		}
	}
	return out
}

func (s *Service) buildSummaryLocked(id string, v *storedSession) Session {
	events := sortedEvents(v)
	meta := map[string]any{}
	for _, e := range events {
		for k, val := range e.Meta {
			if printable(val) != "" {
				meta[k] = val
			}
		}
	}
	for k, val := range v.Meta {
		meta[k] = val
	}
	out := Session{ID: id, Name: text(meta["name"], id), Client: text(meta["client"], "unknown"), Cwd: text(meta["cwd"]), ProjectRoot: text(meta["projectRoot"]), GitRoot: text(meta["gitRoot"]), Branch: text(meta["branch"]), Repository: text(meta["repository"]), Remote: text(meta["remote"]), Status: "unknown", StatusSource: "inferred"}
	traceSet := map[string]bool{}
	var latestState string
	for _, e := range events {
		if e.Model != "" {
			out.Model = e.Model
		}
		if out.Agent == "" {
			out.Agent = e.Agent
		}
		if out.Name == id && e.Role == "user" && e.Content != "" {
			out.Name = e.Content
		}
		if e.Time != "" {
			out.LastActivity = e.Time
			if e.State != "" {
				latestState = e.State
			}
		}
		for _, at := range []string{e.StartTime, e.Time} {
			if at != "" && (out.StartedAt == "" || at < out.StartedAt) {
				out.StartedAt = at
			}
		}
		if e.TraceID != "" {
			traceSet[e.TraceID] = true
		}
	}
	for trace := range traceSet {
		out.TraceIDs = append(out.TraceIDs, trace)
	}
	sort.Strings(out.TraceIDs)
	if latestState != "" && latestState != "active" {
		out.Status = latestState
		out.StatusSource = "confirmed"
	} else if at, err := time.Parse(time.RFC3339Nano, out.LastActivity); err == nil {
		out.Status = "inactive"
		if time.Since(at) < 2*time.Minute {
			out.Status = "active"
		}
	}
	candidates := []string{out.Branch, printable(meta)}
	for _, e := range events {
		if e.Role == "user" {
			candidates = append(candidates, e.Content)
		}
	}
	for _, candidate := range candidates {
		if m := s.pattern.FindString(candidate); m != "" {
			out.Jira = m
			break
		}
	}
	usage := preferredUsage(events)
	for _, e := range usage {
		if e.InputTokens != nil {
			if out.InputTokens == nil {
				out.InputTokens = new(int64)
			}
			*out.InputTokens += *e.InputTokens
		}
		if e.OutputTokens != nil {
			if out.OutputTokens == nil {
				out.OutputTokens = new(int64)
			}
			*out.OutputTokens += *e.OutputTokens
		}
		if e.DurationMs != nil {
			if out.DurationMs == nil {
				out.DurationMs = new(float64)
			}
			*out.DurationMs += *e.DurationMs
		}
	}
	if !s.options.ShowContent {
		out.Name = id
	}
	return out
}
func preferredUsage(events []Event) []Event {
	local, spans, other, points := []Event{}, []Event{}, []Event{}, []Event{}
	bySpan := map[string]Event{}
	for _, e := range events {
		if strings.HasPrefix(e.Key, "span:") {
			bySpan[e.TraceID+":"+e.SpanID] = e
		}
		if e.InputTokens == nil && e.OutputTokens == nil {
			continue
		}
		switch {
		case e.Source == "local" && e.Type == "assistant.usage":
			local = append(local, e)
		case e.Source == "local":
			other = append(other, e)
		case strings.HasPrefix(e.Key, "span:"):
			spans = append(spans, e)
		case strings.HasPrefix(e.Key, "metric:"):
			points = append(points, e)
		}
	}
	if len(local) > 0 {
		return local
	}
	if len(spans) > 0 {
		parents := map[string]bool{}
		for _, e := range spans {
			visited := map[string]bool{}
			for e.Parent != "" {
				key := e.TraceID + ":" + e.Parent
				if visited[key] {
					break
				}
				visited[key] = true
				parents[key] = true
				var ok bool
				e, ok = bySpan[key]
				if !ok {
					break
				}
			}
		}
		leaves := []Event{}
		for _, e := range spans {
			if !parents[e.TraceID+":"+e.SpanID] {
				leaves = append(leaves, e)
			}
		}
		return leaves
	}
	if len(other) > 0 {
		return other
	}
	result := []Event{}
	cumulative := map[string]Event{}
	for _, e := range points {
		if e.Temporality == 1 {
			result = append(result, e)
		} else if e.Temporality == 2 {
			cumulative[e.MetricSeries] = e
		}
	}
	keys := []string{}
	for k := range cumulative {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		result = append(result, cumulative[k])
	}
	return result
}
func (s *Service) Sessions(ctx context.Context) ([]Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := []Session{}
	for id, v := range s.data.Sessions {
		rows = append(rows, s.summaryLocked(id, v))
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].LastActivity != rows[j].LastActivity {
			return rows[i].LastActivity > rows[j].LastActivity
		}
		return rows[i].ID < rows[j].ID
	})
	return rows, nil
}
func (s *Service) Detail(ctx context.Context, id string) (Detail, error) {
	if err := ctx.Err(); err != nil {
		return Detail{}, err
	}
	if err := s.load(); err != nil {
		return Detail{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.data.Sessions[id]
	if v == nil {
		return Detail{}, errors.New("Copilot session not found")
	}
	events := sortedEvents(v)
	dedup := map[string]Event{}
	order := []string{}
	// Local observations win only when a shared event/call identity exists.
	sort.SliceStable(events, func(i, j int) bool { return events[i].Source != "local" && events[j].Source == "local" })
	for _, e := range events {
		key := e.Source + ":" + e.Key
		if e.EventID != "" {
			key = "event:" + e.EventID
		}
		if e.CallID != "" {
			phase := "start"
			if e.Result != "" || strings.Contains(e.Type, "complete") {
				phase = "result"
			}
			key = "tool:" + e.CallID + ":" + phase
		}
		if _, ok := dedup[key]; !ok {
			order = append(order, key)
		}
		dedup[key] = e
	}
	result := Detail{Session: s.summaryLocked(id, v), Events: []Event{}}
	for _, key := range order {
		e := dedup[key]
		if !s.options.ShowContent {
			e = withoutContent(e)
		}
		result.Events = append(result.Events, e)
	}
	sort.SliceStable(result.Events, func(i, j int) bool { return result.Events[i].Time < result.Events[j].Time })
	return result, nil
}

// storageSnapshot materializes the same summaries used by the existing views.
func (s *Service) storageSnapshot() diskStore {
	data := s.data
	data.Summaries = make(map[string]Session, len(data.Sessions))
	for id, session := range data.Sessions {
		data.Summaries[id] = s.summaryLocked(id, session)
	}
	return data
}
