package github

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

// SetAutoMerge resolves the current PR and allowed methods before writing. GitHub
// remains responsible for permissions, branch rules, checks and merge queues.
func (c *Client) SetAutoMerge(ctx context.Context, p model.PullRequest, enabled bool) (model.PullRequest, error) {
	parts := strings.Split(p.Repository, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || p.Number <= 0 {
		return p, errors.New("Ugyldig repository eller PR-nummer")
	}
	var current struct {
		Repository struct {
			MergeCommitAllowed bool
			SquashMergeAllowed bool
			RebaseMergeAllowed bool
			PullRequest        *struct {
				ID      string
				State   string
				IsDraft bool
			}
		}
	}
	err := c.mergeGraphQL(ctx, `query($owner:String!,$repo:String!,$number:Int!){repository(owner:$owner,name:$repo){mergeCommitAllowed squashMergeAllowed rebaseMergeAllowed pullRequest(number:$number){id state isDraft}}}`, map[string]any{"owner": parts[0], "repo": parts[1], "number": p.Number}, &current)
	if err != nil {
		return p, err
	}
	pr := current.Repository.PullRequest
	if pr == nil || pr.ID == "" || pr.State != "OPEN" {
		return p, errors.New("PR-en finnes ikke eller er lukket")
	}
	if enabled && pr.IsDraft {
		return p, errors.New("Gjør PR-en klar for review før du slår på automerge")
	}
	operation := "disablePullRequestAutoMerge"
	input := map[string]any{"pullRequestId": pr.ID}
	inputType := "DisablePullRequestAutoMergeInput!"
	if enabled {
		operation, inputType = "enablePullRequestAutoMerge", "EnablePullRequestAutoMergeInput!"
		method := ""
		switch {
		case current.Repository.MergeCommitAllowed:
			method = "MERGE"
		case current.Repository.SquashMergeAllowed:
			method = "SQUASH"
		case current.Repository.RebaseMergeAllowed:
			method = "REBASE"
		}
		if method == "" {
			return p, errors.New("Repository tillater ingen merge-metode")
		}
		input["mergeMethod"] = method
	}
	var result map[string]struct {
		PullRequest *details `json:"pullRequest"`
	}
	query := "mutation($input:" + inputType + "){ " + operation + "(input:$input){pullRequest{id autoMergeRequest{enabledAt} mergeQueueEntry{position state}}}}"
	if err := c.mergeGraphQL(ctx, query, map[string]any{"input": input}, &result); err != nil {
		return p, err
	}
	updated := result[operation].PullRequest
	if updated == nil || updated.ID != pr.ID {
		return p, errors.New("GitHub returnerte ingen oppdatert automerge-status; bruk au for å oppdatere")
	}
	// Only merge status changed; preserve review, checks and display metadata.
	status := p
	applyDetails(&status, updated)
	p.AutoMerge, p.MergeQueue, p.QueuePosition = status.AutoMerge, status.MergeQueue, status.QueuePosition
	return p, nil
}

func (c *Client) mergeGraphQL(ctx context.Context, query string, variables map[string]any, data any) error {
	req, err := c.api.NewRequest(ctx, "POST", "graphql", map[string]any{"query": query, "variables": variables})
	if err != nil {
		return errors.New("Kunne ikke opprette GitHub-forespørsel")
	}
	var response struct {
		Data   json.RawMessage
		Errors []json.RawMessage
	}
	if _, err := c.api.Do(req, &response); err != nil {
		return safeError(err)
	}
	if len(response.Errors) > 0 {
		return errors.New("GitHub kunne ikke endre automerge; kontroller skrivetilgang, repository-innstillinger og branch-regler. Bruk au for å hente status på nytt")
	}
	if len(response.Data) == 0 || string(response.Data) == "null" || json.Unmarshal(response.Data, data) != nil {
		return errors.New("Kunne ikke lese GitHub automerge-status")
	}
	return nil
}
