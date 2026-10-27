package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"

	"github.com/haakonph/custom-visning-terminal/internal/localstate"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/relations"
	"github.com/haakonph/custom-visning-terminal/internal/storage/dbgen"
)

type LocalStore struct{ Path string }

func (s LocalStore) CommitLocal(ctx context.Context, changes localstate.Changes) (localstate.Result, error) {
	result := localstate.Result{}
	db, err := open(s.Path)
	if err != nil {
		return result, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if err = applyTasks(ctx, tx, changes.Tasks); err != nil {
		return result, err
	}
	if err = applyRelations(ctx, tx, changes.Relations); err != nil {
		return result, err
	}
	if err = applySnoozes(ctx, tx, changes.Snoozes); err != nil {
		return result, err
	}
	q := dbgen.New(tx)
	if err = readDomain(ctx, q, &result.Tasks); err != nil {
		return result, err
	}
	if err = readDomain(ctx, q, &result.Relations); err != nil {
		return result, err
	}
	if err = readDomain(ctx, q, &result.Snoozes); err != nil {
		return result, err
	}
	if err = tx.Commit(); err != nil {
		return localstate.Result{}, err
	}
	return result, nil
}

var ErrLocalConflict = errors.New("lokale data er endret i en annen appinstans; last inn på nytt")

func applyRelations(ctx context.Context, tx *sql.Tx, changes relations.Changes) error {
	if len(changes.Links) == 0 && len(changes.Archive) == 0 {
		return nil
	}
	q := dbgen.New(tx)
	var current model.EntityState
	if err := readEntities(ctx, q, &current); err != nil {
		return err
	}
	edges := map[string]model.EntityLink{}
	archived := map[string]model.TaskReference{}
	for _, e := range current.Links {
		edges[relations.LinkID(e)] = e
	}
	for _, r := range current.Archived {
		archived[relations.ReferenceID(r)] = r
	}
	for _, c := range changes.Links {
		if c.Before == nil && c.After == nil {
			return errors.New("tom koblingsendring")
		}
		var e model.EntityLink
		if c.Before != nil {
			e = *c.Before
		} else {
			e = *c.After
		}
		id := relations.LinkID(e)
		actual, exists := edges[id]
		if c.Before != nil && (!exists || !reflect.DeepEqual(actual, *c.Before)) {
			return fmt.Errorf("%w: kobling", ErrLocalConflict)
		}
		if c.Before == nil && exists {
			if reflect.DeepEqual(actual, *c.After) {
				continue
			}
			return ErrLocalConflict
		}
		if c.Before != nil {
			if _, err := tx.ExecContext(ctx, "DELETE FROM entity_links WHERE source_provider=? AND source_url=? AND target_provider=? AND target_url=?", e.Left.Provider, e.Left.URL, e.Right.Provider, e.Right.URL); err != nil {
				return err
			}
			delete(edges, id)
		}
		if c.After != nil {
			e = *c.After
			if err := q.InsertEntityLink(ctx, dbgen.InsertEntityLinkParams{SourceProvider: e.Left.Provider, SourceUrl: e.Left.URL, SourceKey: e.Left.Key, SourceTitle: e.Left.Title, TargetProvider: e.Right.Provider, TargetUrl: e.Right.URL, TargetKey: e.Right.Key, TargetTitle: e.Right.Title}); err != nil {
				return err
			}
			edges[relations.LinkID(e)] = e
		}
	}
	for _, c := range changes.Archive {
		if c.Before == nil && c.After == nil {
			return errors.New("tom arkivendring")
		}
		var r model.TaskReference
		if c.Before != nil {
			r = *c.Before
		} else {
			r = *c.After
		}
		id := relations.ReferenceID(r)
		actual, exists := archived[id]
		if c.Before != nil && (!exists || !reflect.DeepEqual(actual, *c.Before)) {
			return fmt.Errorf("%w: arkiv", ErrLocalConflict)
		}
		if c.Before == nil && exists {
			if reflect.DeepEqual(actual, *c.After) {
				continue
			}
			return ErrLocalConflict
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM archived_entities WHERE provider=? AND url=?", r.Provider, r.URL); err != nil {
			return err
		}
		delete(archived, id)
		if c.After != nil {
			r = *c.After
			if err := q.InsertArchivedEntity(ctx, dbgen.InsertArchivedEntityParams{Provider: r.Provider, Url: r.URL, ReferenceKey: r.Key, Title: r.Title}); err != nil {
				return err
			}
			archived[relations.ReferenceID(r)] = r
		}
	}
	return q.SetState(ctx, "entities")
}
func applySnoozes(ctx context.Context, tx *sql.Tx, changes []localstate.SnoozeChange) error {
	if len(changes) == 0 {
		return nil
	}
	q := dbgen.New(tx)
	var current map[string]model.SnoozeEntry
	if err := readDomain(ctx, q, &current); err != nil {
		return err
	}
	for _, c := range changes {
		if c.Before == nil && c.After == nil {
			return errors.New("tom utsettelse")
		}
		id := ""
		if c.Before != nil {
			id = c.Before.ID
		} else {
			id = c.After.ID
		}
		actual, exists := current[id]
		if c.Before != nil && (!exists || !reflect.DeepEqual(actual, *c.Before)) {
			return fmt.Errorf("%w: utsettelse", ErrLocalConflict)
		}
		if c.Before == nil && exists {
			if reflect.DeepEqual(actual, *c.After) {
				continue
			}
			return ErrLocalConflict
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM snoozes WHERE id=?", id); err != nil {
			return err
		}
		delete(current, id)
		if c.After == nil {
			continue
		}
		e := *c.After
		if e.ID != id || id == "" || e.Until.IsZero() || e.Until.Year() > 9999 || (e.Kind != "github" && e.Kind != "jira" && e.Kind != "task" && e.Kind != "copilot" && e.Kind != "mattermost") {
			return errors.New("ugyldig utsettelse")
		}
		if err := q.InsertSnoozes(ctx, dbgen.InsertSnoozesParams{ID: e.ID, Kind: e.Kind, Title: e.Title, Url: e.URL, UntilAt: timestamp(e.Until)}); err != nil {
			return err
		}
		for role, refs := range map[string][]model.TaskReference{"root": e.Roots, "member": e.Members} {
			for i, r := range refs {
				if err := q.InsertSnoozeReference(ctx, dbgen.InsertSnoozeReferenceParams{SnoozeID: e.ID, ReferenceRole: role, Position: int64(i), Provider: r.Provider, Url: r.URL, ReferenceKey: r.Key, Title: r.Title}); err != nil {
					return err
				}
			}
		}
		current[id] = e
	}
	return q.SetState(ctx, "snoozes")
}

// LoadLocalState reads the domains from one consistent SQLite snapshot.
func (s LocalStore) LoadLocalState(ctx context.Context) (localstate.Result, error) {
	result := localstate.Result{}
	db, err := open(s.Path)
	if err != nil {
		return result, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	q := dbgen.New(tx)
	if err = readDomain(ctx, q, &result.Tasks); err != nil {
		return result, err
	}
	if err = readDomain(ctx, q, &result.Relations); err != nil {
		return result, err
	}
	if err = readDomain(ctx, q, &result.Snoozes); err != nil {
		return result, err
	}
	return result, tx.Commit()
}
