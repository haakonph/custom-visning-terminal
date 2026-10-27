// Package app coordinates commands that span domains. Domain state and rules
// remain in tasks, relations, pullrequests, jira, conversations and copilot.
package app

import (
	"context"
	"github.com/haakonph/custom-visning-terminal/internal/localstate"
)

type LocalChanges = localstate.Changes
type LocalResult = localstate.Result
type LocalRepository interface {
	CommitLocal(context.Context, LocalChanges) (LocalResult, error)
}

// Commit preserves one transaction across domain-owned tables. It deliberately
// contains no networking or terminal operations.
func Commit(ctx context.Context, repo LocalRepository, changes LocalChanges) (LocalResult, error) {
	return repo.CommitLocal(ctx, changes)
}
