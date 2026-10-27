// Package localstate defines the transaction boundary shared by local domains.
package localstate

import (
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/relations"
	"github.com/haakonph/custom-visning-terminal/internal/tasks"
)

type SnoozeChange struct{ Before, After *model.SnoozeEntry }
type Changes struct {
	Tasks     []tasks.Change
	Relations relations.Changes
	Snoozes   []SnoozeChange
}
type Result struct {
	Tasks     []model.Task
	Relations model.EntityState
	Snoozes   map[string]model.SnoozeEntry
}
