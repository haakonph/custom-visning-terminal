package app

import (
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/tasks"
	"testing"
)

func TestLinkArchiveAndDeleteCoordinateDomainRules(t *testing.T) {
	task := model.Task{ID: "work", Title: "Work"}
	owner := tasks.Reference(task)
	pr := model.TaskReference{Provider: "github", URL: "https://github.com/o/r/pull/1", Title: "PR"}
	state, items, changed := LinkReferences(model.EntityState{}, []model.Task{task}, []model.TaskReference{owner, pr}, []model.TaskReference{pr, owner})
	if !changed || len(state.Links) != 1 || len(items[0].References) != 1 || len(task.References) != 0 {
		t.Fatal("linking duplicated an edge or mutated its inputs")
	}
	state, items, changed = ArchiveReferences(state, items, []model.TaskReference{owner, pr}, true)
	if !changed || !items[0].Archived || len(state.Archived) != 1 {
		t.Fatal("archive did not coordinate local domains")
	}
	state, items = DeleteReferences(state, items, []model.TaskReference{pr}, task.ID)
	if len(items) != 1 || len(items[0].References) != 0 || len(state.Links) != 0 {
		t.Fatal("reference deletion deleted its owner or retained the edge")
	}
}
