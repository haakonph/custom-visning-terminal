package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/storage/dbgen"
)

func intNull(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}
func floatNull(v *float64) sql.NullFloat64 {
	if v == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *v, Valid: true}
}
func intPointer(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}
func floatPointer(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	return &v.Float64
}
func jsonText(v any) (string, error) { b, err := json.Marshal(v); return string(b), err }

func writeCopilot(ctx context.Context, q *dbgen.Queries, data model.CopilotStore) error {
	if err := q.ClearCopilotSessions(ctx); err != nil {
		return err
	}
	return writeCopilotChanges(ctx, q, data)
}

// SaveCopilotChanges atomically upserts the supplied sessions, events and trace
// mappings. Omitted rows are preserved; only explicitly removed sessions are
// deleted. Callers must migrate any legacy store before applying changes.
func SaveCopilotChanges(path string, data model.CopilotStore, removed []string) error {
	if len(data.Sessions) == 0 && len(data.Traces) == 0 && len(removed) == 0 {
		return nil
	}
	db, err := open(path)
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ctx := context.Background()
	q := dbgen.New(tx)
	// A removed trace session may have been recreated before this flush. Delete
	// its old rows first; the changes below contain all moved or recreated data.
	for _, id := range removed {
		if err := q.DeleteCopilotSession(ctx, id); err != nil {
			return err
		}
	}
	if err := writeCopilotChanges(ctx, q, data); err != nil {
		return err
	}
	if err := q.SetState(ctx, "copilot"); err != nil {
		return err
	}
	return tx.Commit()
}

func writeCopilotChanges(ctx context.Context, q *dbgen.Queries, data model.CopilotStore) error {
	if data.Version != 1 || data.Sessions == nil || data.Traces == nil {
		return errors.New("invalid Copilot store")
	}
	for id, s := range data.Sessions {
		if s == nil || s.Meta == nil || s.Events == nil {
			return errors.New("invalid Copilot session")
		}
		metadata, err := jsonText(s.Meta)
		if err != nil {
			return err
		}
		summary := data.Summaries[id]
		if err = q.InsertCopilotSessions(ctx, dbgen.InsertCopilotSessionsParams{ID: id, Name: summary.Name, Client: summary.Client, Cwd: summary.Cwd, ProjectRoot: summary.ProjectRoot, GitRoot: summary.GitRoot, Branch: summary.Branch, Repository: summary.Repository, Remote: summary.Remote, ExtraMetadata: metadata, Model: summary.Model, Agent: summary.Agent, Jira: summary.Jira, StartedAt: summary.StartedAt, LastActivity: summary.LastActivity, Status: summary.Status, StatusSource: summary.StatusSource, InputTokens: intNull(summary.InputTokens), OutputTokens: intNull(summary.OutputTokens), DurationMs: floatNull(summary.DurationMs)}); err != nil {
			return err
		}
		for key, e := range s.Events {
			metadata, err := jsonText(e.Meta)
			if err != nil {
				return err
			}
			raw := ""
			if len(e.Raw) > 0 {
				if !json.Valid(e.Raw) {
					return errors.New("invalid Copilot raw event JSON")
				}
				raw = string(e.Raw)
			}
			row := dbgen.InsertCopilotEventsParams{SessionID: id, EventKey: key, SourceKey: e.Key, Source: e.Source, ObservedSessionID: e.SessionID, TraceID: e.TraceID, SpanID: e.SpanID, EventID: e.EventID, StartTime: e.StartTime, State: e.State, MetricSeries: e.MetricSeries, Time: e.Time, EventType: e.Type, Content: e.Content, Role: e.Role, Tool: e.Tool, CallID: e.CallID, Args: e.Args, Result: e.Result, Error: e.Error, Warning: e.Warning, Agent: e.Agent, Parent: e.Parent, Model: e.Model, Metadata: metadata, Raw: raw, Temporality: int64(e.Temporality)}
			if err = q.InsertCopilotEvents(ctx, row); err != nil {
				return err
			}
			if e.InputTokens != nil || e.OutputTokens != nil || e.DurationMs != nil {
				if err = q.InsertCopilotUsage(ctx, dbgen.InsertCopilotUsageParams{SessionID: id, EventKey: key, InputTokens: intNull(e.InputTokens), OutputTokens: intNull(e.OutputTokens), DurationMs: floatNull(e.DurationMs)}); err != nil {
					return err
				}
			} else if err := q.DeleteCopilotEventUsage(ctx, dbgen.DeleteCopilotEventUsageParams{SessionID: id, EventKey: key}); err != nil {
				return err
			}
		}
	}
	for trace, session := range data.Traces {
		if err := q.InsertCopilotTraces(ctx, dbgen.InsertCopilotTracesParams{TraceID: trace, SessionID: session}); err != nil {
			return err
		}
	}
	return nil
}

func readCopilot(ctx context.Context, q *dbgen.Queries) (model.CopilotStore, error) {
	out := model.CopilotStore{Version: 1, Sessions: map[string]*model.CopilotStoredSession{}, Traces: map[string]string{}, Summaries: map[string]model.CopilotSession{}}
	sessions, err := q.ListCopilotSessions(ctx)
	if err != nil {
		return out, err
	}
	for _, r := range sessions {
		s := &model.CopilotStoredSession{Events: map[string]model.CopilotEvent{}}
		if err = json.Unmarshal([]byte(r.ExtraMetadata), &s.Meta); err != nil {
			return out, err
		}
		out.Sessions[r.ID] = s
		out.Summaries[r.ID] = model.CopilotSession{ID: r.ID, Name: r.Name, Client: r.Client, Cwd: r.Cwd, ProjectRoot: r.ProjectRoot, GitRoot: r.GitRoot, Branch: r.Branch, Repository: r.Repository, Remote: r.Remote, Model: r.Model, Agent: r.Agent, Jira: r.Jira, StartedAt: r.StartedAt, LastActivity: r.LastActivity, Status: r.Status, StatusSource: r.StatusSource, InputTokens: intPointer(r.InputTokens), OutputTokens: intPointer(r.OutputTokens), DurationMs: floatPointer(r.DurationMs)}
	}
	events, err := q.ListCopilotEvents(ctx)
	if err != nil {
		return out, err
	}
	for _, r := range events {
		e := model.CopilotEvent{SessionID: r.ObservedSessionID, Key: r.SourceKey, Source: r.Source, TraceID: r.TraceID, SpanID: r.SpanID, EventID: r.EventID, StartTime: r.StartTime, State: r.State, MetricSeries: r.MetricSeries, Time: r.Time, Type: r.EventType, Content: r.Content, Role: r.Role, Tool: r.Tool, CallID: r.CallID, Args: r.Args, Result: r.Result, Error: r.Error, Warning: r.Warning, Agent: r.Agent, Parent: r.Parent, Model: r.Model, Temporality: int(r.Temporality)}
		if err = json.Unmarshal([]byte(r.Metadata), &e.Meta); err != nil {
			return out, err
		}
		if r.Raw != "" {
			if !json.Valid([]byte(r.Raw)) {
				return out, errors.New("invalid raw event data")
			}
			e.Raw = json.RawMessage(r.Raw)
		}
		if out.Sessions[r.SessionID] == nil {
			return out, errors.New("missing Copilot session")
		}
		out.Sessions[r.SessionID].Events[r.EventKey] = e
	}
	usage, err := q.ListCopilotUsage(ctx)
	if err != nil {
		return out, err
	}
	for _, r := range usage {
		s := out.Sessions[r.SessionID]
		if s == nil {
			return out, errors.New("missing Copilot session")
		}
		e, ok := s.Events[r.EventKey]
		if !ok {
			return out, errors.New("missing Copilot event")
		}
		e.InputTokens = intPointer(r.InputTokens)
		e.OutputTokens = intPointer(r.OutputTokens)
		e.DurationMs = floatPointer(r.DurationMs)
		s.Events[r.EventKey] = e
	}
	traces, err := q.ListCopilotTraces(ctx)
	if err != nil {
		return out, err
	}
	for _, r := range traces {
		if out.Sessions[r.SessionID] == nil {
			return out, errors.New("missing Copilot session")
		}
		out.Traces[r.TraceID] = r.SessionID
	}
	return out, nil
}
