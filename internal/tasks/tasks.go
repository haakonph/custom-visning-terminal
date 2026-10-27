// Package tasks owns local tasks and optimistic editing commands.
package tasks

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"reflect"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

var ErrConflict = errors.New("oppgaven er endret i en annen appinstans; last inn på nytt før du lagrer")

type Change struct{ Before, After *model.Task }
type Repository interface {
	ApplyTaskChanges(context.Context, []Change) ([]model.Task, error)
}

func Clone(items []model.Task) []model.Task {
	out := append([]model.Task(nil), items...)
	for i := range out {
		out[i].References = append([]model.TaskReference(nil), out[i].References...)
	}
	return out
}
func EnsureIDs(items []model.Task) error {
	seen := map[string]bool{}
	for i := range items {
		if items[i].ID == "" {
			var id [16]byte
			if _, err := rand.Read(id[:]); err != nil {
				return err
			}
			items[i].ID = fmt.Sprintf("%x", id)
		}
		if seen[items[i].ID] {
			return fmt.Errorf("duplisert oppgave-ID: %s", items[i].ID)
		}
		seen[items[i].ID] = true
	}
	return nil
}

// Changes converts an editor draft into targeted operations. Unchanged tasks
// never participate in the write, so independent edits can commit concurrently.
func Changes(before, after []model.Task) ([]Change, error) {
	old := map[string]model.Task{}
	for _, t := range before {
		if t.ID == "" {
			return nil, fmt.Errorf("oppgaven mangler ID")
		}
		old[t.ID] = t
	}
	var changes []Change
	seen := map[string]bool{}
	for _, t := range after {
		if t.ID == "" || seen[t.ID] {
			return nil, fmt.Errorf("ugyldig oppgave-ID: %s", t.ID)
		}
		seen[t.ID] = true
		previous, exists := old[t.ID]
		if exists && reflect.DeepEqual(previous, t) {
			continue
		}
		next := Clone([]model.Task{t})[0]
		c := Change{After: &next}
		if exists {
			v := Clone([]model.Task{previous})[0]
			c.Before = &v
		}
		changes = append(changes, c)
	}
	for _, t := range before {
		if !seen[t.ID] {
			v := Clone([]model.Task{t})[0]
			changes = append(changes, Change{Before: &v})
		}
	}
	return changes, nil
}

func Create(ctx context.Context, repo Repository, task model.Task) ([]model.Task, error) {
	items := []model.Task{task}
	if err := EnsureIDs(items); err != nil {
		return nil, err
	}
	return repo.ApplyTaskChanges(ctx, []Change{{After: &items[0]}})
}
func Update(ctx context.Context, repo Repository, before, after model.Task) ([]model.Task, error) {
	if before.ID == "" || before.ID != after.ID {
		return nil, fmt.Errorf("oppgave-ID kan ikke endres")
	}
	return repo.ApplyTaskChanges(ctx, []Change{{Before: &before, After: &after}})
}
func Delete(ctx context.Context, repo Repository, task model.Task) ([]model.Task, error) {
	return repo.ApplyTaskChanges(ctx, []Change{{Before: &task}})
}
