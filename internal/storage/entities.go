package storage

import (
	"context"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/storage/dbgen"
)

func writeEntities(ctx context.Context, q *dbgen.Queries, state model.EntityState) error {
	if err := q.ClearEntityLinks(ctx); err != nil {
		return err
	}
	if err := q.ClearArchivedEntities(ctx); err != nil {
		return err
	}
	for _, e := range state.Links {
		if err := q.InsertEntityLink(ctx, dbgen.InsertEntityLinkParams{SourceProvider: e.Left.Provider, SourceUrl: e.Left.URL, SourceKey: e.Left.Key, SourceTitle: e.Left.Title, TargetProvider: e.Right.Provider, TargetUrl: e.Right.URL, TargetKey: e.Right.Key, TargetTitle: e.Right.Title}); err != nil {
			return err
		}
	}
	for _, r := range state.Archived {
		if err := q.InsertArchivedEntity(ctx, dbgen.InsertArchivedEntityParams{Provider: r.Provider, Url: r.URL, ReferenceKey: r.Key, Title: r.Title}); err != nil {
			return err
		}
	}
	return nil
}
func readEntities(ctx context.Context, q *dbgen.Queries, state *model.EntityState) error {
	edges, err := q.ListEntityLinks(ctx)
	if err != nil {
		return err
	}
	archived, err := q.ListArchivedEntities(ctx)
	if err != nil {
		return err
	}
	*state = model.EntityState{}
	for _, e := range edges {
		state.Links = append(state.Links, model.EntityLink{Left: model.TaskReference{Provider: e.SourceProvider, URL: e.SourceUrl, Key: e.SourceKey, Title: e.SourceTitle}, Right: model.TaskReference{Provider: e.TargetProvider, URL: e.TargetUrl, Key: e.TargetKey, Title: e.TargetTitle}})
	}
	for _, r := range archived {
		state.Archived = append(state.Archived, model.TaskReference{Provider: r.Provider, URL: r.Url, Key: r.ReferenceKey, Title: r.Title})
	}
	return nil
}
