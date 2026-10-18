package storage

import (
	"context"
	"fmt"

	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/storage/dbgen"
)

func writeDomain(ctx context.Context, q *dbgen.Queries, value any) error {
	switch v := value.(type) {
	case model.Snapshot:
		return writeGitHub(ctx, q, v)
	case model.JiraCache:
		return writeJira(ctx, q, v)
	case model.CopilotStore:
		return writeCopilot(ctx, q, v)
	case []model.Task:
		if err := q.ClearTasks(ctx); err != nil {
			return err
		}
		for pos, t := range v {
			if err := q.InsertTasks(ctx, dbgen.InsertTasksParams{Position: int64(pos), StableID: t.ID, Title: t.Title}); err != nil {
				return err
			}
			for refPos, r := range t.References {
				if err := q.InsertTaskReferences(ctx, dbgen.InsertTaskReferencesParams{TaskPosition: int64(pos), Position: int64(refPos), Provider: r.Provider, Url: r.URL, ReferenceKey: r.Key, Title: r.Title, Role: r.Role}); err != nil {
					return err
				}
			}
		}
	case map[string]string:
		if err := q.ClearLocalRepositories(ctx); err != nil {
			return err
		}
		for key, path := range v {
			if err := q.InsertLocalRepositories(ctx, dbgen.InsertLocalRepositoriesParams{Identity: key, Path: path}); err != nil {
				return err
			}
		}
	case map[string]model.LocalStatus:
		if err := q.ClearLocalWorktrees(ctx); err != nil {
			return err
		}
		for key, status := range v {
			if err := q.InsertLocalWorktrees(ctx, dbgen.InsertLocalWorktreesParams{Identity: key, Path: status.Path, Head: status.Head}); err != nil {
				return err
			}
		}
	case map[string]model.SnoozeEntry:
		if err := q.ClearSnoozes(ctx); err != nil {
			return err
		}
		for key, e := range v {
			if key == "" || e.ID != key || e.Until.IsZero() || e.Until.Year() > 9999 || (e.Kind != "github" && e.Kind != "jira" && e.Kind != "task") {
				return fmt.Errorf("invalid snooze entry")
			}
			if err := q.InsertSnoozes(ctx, dbgen.InsertSnoozesParams{ID: e.ID, Kind: e.Kind, Title: e.Title, Url: e.URL, UntilAt: timestamp(e.Until)}); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported storage type %T", value)
	}
	return nil
}

func readDomain(ctx context.Context, q *dbgen.Queries, value any) error {
	switch out := value.(type) {
	case *model.Snapshot:
		v, err := readGitHub(ctx, q)
		if err != nil {
			return err
		}
		*out = v
	case *model.JiraCache:
		v, err := readJira(ctx, q)
		if err != nil {
			return err
		}
		*out = v
	case *model.CopilotStore:
		v, err := readCopilot(ctx, q)
		if err != nil {
			return err
		}
		*out = v
	case *[]model.Task:
		rows, err := q.ListTasks(ctx)
		if err != nil {
			return err
		}
		items := make([]model.Task, len(rows))
		indices := map[int64]int{}
		for i, r := range rows {
			items[i] = model.Task{ID: r.StableID, Title: r.Title}
			indices[r.Position] = i
		}
		refs, err := q.ListTaskReferences(ctx)
		if err != nil {
			return err
		}
		for _, r := range refs {
			i, ok := indices[r.TaskPosition]
			if !ok {
				return fmt.Errorf("missing task for reference")
			}
			items[i].References = append(items[i].References, model.TaskReference{Provider: r.Provider, URL: r.Url, Key: r.ReferenceKey, Title: r.Title, Role: r.Role})
		}
		*out = items
	case *map[string]string:
		rows, err := q.ListLocalRepositories(ctx)
		if err != nil {
			return err
		}
		items := map[string]string{}
		for _, r := range rows {
			items[r.Identity] = r.Path
		}
		*out = items
	case *map[string]model.LocalStatus:
		rows, err := q.ListLocalWorktrees(ctx)
		if err != nil {
			return err
		}
		items := map[string]model.LocalStatus{}
		for _, r := range rows {
			items[r.Identity] = model.LocalStatus{Path: r.Path, Head: r.Head}
		}
		*out = items
	case *map[string]model.SnoozeEntry:
		rows, err := q.ListSnoozes(ctx)
		if err != nil {
			return err
		}
		items := map[string]model.SnoozeEntry{}
		for _, r := range rows {
			at, err := parseTime(r.UntilAt)
			if err != nil {
				return err
			}
			items[r.ID] = model.SnoozeEntry{ID: r.ID, Kind: r.Kind, Title: r.Title, URL: r.Url, Until: at}
		}
		*out = items
	default:
		return fmt.Errorf("unsupported storage destination %T", value)
	}
	return nil
}
