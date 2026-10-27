package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"

	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/storage/dbgen"
	"github.com/haakonph/custom-visning-terminal/internal/tasks"
)

type TaskStore struct{ Path string }

func (s TaskStore) ApplyTaskChanges(ctx context.Context, changes []tasks.Change) ([]model.Task, error) {
	db, err := open(s.Path)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = applyTasks(ctx, tx, changes); err != nil {
		return nil, err
	}
	var result []model.Task
	if err = readDomain(ctx, dbgen.New(tx), &result); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}
func applyTasks(ctx context.Context, tx *sql.Tx, changes []tasks.Change) error {
	if len(changes) == 0 {
		return nil
	}
	q := dbgen.New(tx)
	var current []model.Task
	if err := readDomain(ctx, q, &current); err != nil {
		return err
	}
	byID := map[string]model.Task{}
	for _, t := range current {
		byID[t.ID] = t
	}
	for _, c := range changes {
		if c.Before == nil && c.After == nil {
			return errors.New("tom oppgaveendring")
		}
		id := ""
		if c.After != nil {
			id = c.After.ID
		} else {
			id = c.Before.ID
		}
		if id == "" || (c.Before != nil && c.Before.ID != id) {
			return errors.New("ugyldig oppgave-ID")
		}
		actual, exists := byID[id]
		if c.Before == nil {
			if exists {
				return fmt.Errorf("%w: %s", tasks.ErrConflict, id)
			}
		} else if !exists || actual.Revision != c.Before.Revision || !reflect.DeepEqual(actual, *c.Before) {
			return fmt.Errorf("%w: %s", tasks.ErrConflict, id)
		}
		var position int64
		if exists {
			if err := tx.QueryRowContext(ctx, "SELECT position FROM tasks WHERE stable_id=?", id).Scan(&position); err != nil {
				return err
			}
		}
		if c.After == nil {
			if _, err := tx.ExecContext(ctx, "DELETE FROM tasks WHERE stable_id=? AND revision=?", id, c.Before.Revision); err != nil {
				return err
			}
			delete(byID, id)
			continue
		}
		next := *c.After
		if !exists {
			if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(position),-1)+1 FROM tasks").Scan(&position); err != nil {
				return err
			}
			next.Revision = 1
			if _, err := tx.ExecContext(ctx, "INSERT INTO tasks(position,stable_id,title,archived,revision) VALUES(?,?,?,?,?)", position, id, next.Title, boolInt(next.Archived), next.Revision); err != nil {
				return err
			}
		} else {
			next.Revision = actual.Revision + 1
			if _, err := tx.ExecContext(ctx, "UPDATE tasks SET title=?,archived=?,revision=? WHERE stable_id=? AND revision=?", next.Title, boolInt(next.Archived), next.Revision, id, actual.Revision); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM task_references WHERE task_position=?", position); err != nil {
				return err
			}
		}
		for i, r := range next.References {
			if err := q.InsertTaskReferences(ctx, dbgen.InsertTaskReferencesParams{TaskPosition: position, Position: int64(i), Provider: r.Provider, Url: r.URL, ReferenceKey: r.Key, Title: r.Title, Role: r.Role}); err != nil {
				return err
			}
		}
		byID[id] = next
	}
	return q.SetState(ctx, "tasks")
}
