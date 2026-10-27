package app

import (
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/relations"
	"github.com/haakonph/custom-visning-terminal/internal/tasks"
)

// These workflows coordinate domain rules; the TUI only chooses their inputs.
// Their effects are committed together through LocalRepository.
func LinkReferences(state model.EntityState, items []model.Task, origins, targets []model.TaskReference) (model.EntityState, []model.Task, bool) {
	return relations.Link(state, origins, targets), tasks.LinkReferences(items, origins, targets), tasks.HasTasks(origins) || tasks.HasTasks(targets)
}
func ArchiveReferences(state model.EntityState, items []model.Task, refs []model.TaskReference, archived bool) (model.EntityState, []model.Task, bool) {
	return relations.Archive(state, refs, archived), tasks.Archive(items, refs, archived), tasks.HasTasks(refs)
}
func DeleteReferences(state model.EntityState, items []model.Task, refs []model.TaskReference, ownerID string) (model.EntityState, []model.Task) {
	return relations.Unlink(state, refs, ownerID), tasks.RemoveReferences(items, refs, ownerID)
}
