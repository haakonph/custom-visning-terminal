package github

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestReviewFallbackAndMultipleReviewers(t *testing.T) {
	opinion := func(login, state string) map[string]any {
		return map[string]any{"state": state, "author": map[string]any{"login": login}}
	}
	request := func(login string) map[string]any {
		return map[string]any{"requestedReviewer": map[string]any{"login": login}}
	}
	team := map[string]any{"requestedReviewer": map[string]any{"slug": "dev", "organization": map[string]any{"login": "acme"}}}
	a := opinion("alice", "APPROVED")
	b := opinion("bob", "APPROVED")
	changes := opinion("bob", "CHANGES_REQUESTED")
	for _, tc := range []struct {
		name, decision                    string
		opinions, requests                []any
		review                            model.Status
		approved, changes, pending, teams int
	}{
		{name: "two approvals", opinions: []any{a, b}, review: model.Approved, approved: 2},
		{name: "changes win", opinions: []any{a, changes}, review: model.ChangesRequested, approved: 1, changes: 1},
		{name: "comment preserves approval", opinions: []any{a, opinion("alice", "COMMENTED")}, review: model.Approved, approved: 1},
		{name: "comments only", opinions: []any{opinion("alice", "COMMENTED")}, review: model.NoReview},
		{name: "no reviews", review: model.NoReview},
		{name: "awaiting review", requests: []any{request("carol")}, review: model.Pending, pending: 1},
		{name: "awaiting team", requests: []any{team}, review: model.Pending, teams: 1},
		{name: "changed to approval", opinions: []any{changes, b}, review: model.Approved, approved: 1},
		{name: "changed to changes", opinions: []any{b, changes}, review: model.ChangesRequested, changes: 1},
		{name: "dismissed", opinions: []any{b, opinion("bob", "DISMISSED")}, review: model.NoReview},
		{name: "case insensitive dedup", opinions: []any{b, opinion("Bob", "APPROVED")}, requests: []any{request("Carol"), request("carol")}, review: model.Approved, approved: 1, pending: 1},
		{name: "re-review pending separately", opinions: []any{a}, requests: []any{request("alice")}, review: model.Approved, approved: 1, pending: 1},
		{name: "GitHub requirements win", decision: "REVIEW_REQUIRED", opinions: []any{a, b}, review: model.Pending, approved: 2},
		{name: "GitHub approval wins", decision: "APPROVED", opinions: []any{changes}, review: model.Approved, changes: 1},
		{name: "GitHub changes win", decision: "CHANGES_REQUESTED", opinions: []any{a}, review: model.ChangesRequested, approved: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.opinions == nil {
				tc.opinions = []any{}
			}
			if tc.requests == nil {
				tc.requests = []any{}
			}
			data := map[string]any{"reviewDecision": tc.decision,
				"latestOpinionatedReviews": map[string]any{"nodes": tc.opinions, "pageInfo": map[string]any{"hasNextPage": false}},
				"reviewRequests":           map[string]any{"nodes": tc.requests, "pageInfo": map[string]any{"hasNextPage": false}}}
			raw, _ := json.Marshal(data)
			var d details
			if err := json.Unmarshal(raw, &d); err != nil {
				t.Fatal(err)
			}
			p := model.PullRequest{Review: model.Approved} // Never retain a stale opinion.
			applyDetails(&p, &d)
			s := p.ReviewSummary
			if p.Review != tc.review || s == nil {
				t.Fatalf("review=%s summary=%+v", p.Review, s)
			}
			if s.Approved != tc.approved || s.Changes != tc.changes || s.Pending != tc.pending || s.PendingTeams != tc.teams || !s.Complete || s.Fallback != (tc.decision == "") {
				t.Fatalf("summary=%+v", s)
			}
		})
	}
}

func TestReviewFallbackUnavailableOrTruncated(t *testing.T) {
	for _, raw := range []string{
		`{}`,
		`{"latestOpinionatedReviews":null}`,
		`{"latestOpinionatedReviews":{"nodes":[],"pageInfo":{"hasNextPage":true}},"reviewRequests":{"nodes":[],"pageInfo":{"hasNextPage":false}}}`,
		`{"latestOpinionatedReviews":{"nodes":[],"pageInfo":{"hasNextPage":false}},"reviewRequests":{"nodes":[],"pageInfo":{"hasNextPage":true}}}`,
		`{"latestOpinionatedReviews":{"nodes":[],"pageInfo":{"hasNextPage":false}}}`,
		`{"latestOpinionatedReviews":{"nodes":[null],"pageInfo":{"hasNextPage":false}},"reviewRequests":{"nodes":[],"pageInfo":{"hasNextPage":false}}}`,
		`{"latestOpinionatedReviews":{"nodes":[{"state":"APPROVED","author":null}],"pageInfo":{"hasNextPage":false}},"reviewRequests":{"nodes":[],"pageInfo":{"hasNextPage":false}}}`,
		`{"latestOpinionatedReviews":{"nodes":null,"pageInfo":{"hasNextPage":false}},"reviewRequests":{"nodes":[],"pageInfo":{"hasNextPage":false}}}`,
		`{"latestOpinionatedReviews":{"nodes":[],"pageInfo":null},"reviewRequests":{"nodes":[],"pageInfo":{"hasNextPage":false}}}`,
	} {
		t.Run(raw, func(t *testing.T) {
			var d details
			if err := json.Unmarshal([]byte(raw), &d); err != nil {
				t.Fatal(err)
			}
			p := model.PullRequest{Review: model.Approved}
			applyDetails(&p, &d)
			if p.Review != model.Unknown || p.ReviewSummary != nil && p.ReviewSummary.Complete {
				t.Fatalf("%+v", p)
			}
			d.ReviewDecision = "APPROVED"
			applyDetails(&p, &d)
			if p.Review != model.Approved {
				t.Fatal("discarded GitHub decision")
			}
		})
	}
}

func TestReviewFallbackEnrichment(t *testing.T) {
	for _, truncated := range []bool{false, true} {
		client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				Query string `json:"query"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Error(err)
			}
			if !strings.Contains(req.Query, "latestOpinionatedReviews(first:100)") || !strings.Contains(req.Query, "nodes { state author { login } }") {
				t.Error("missing opinionated review query")
			}
			writeJSON(w, map[string]any{"data": map[string]any{"nodes": []any{map[string]any{
				"id": "N1", "reviewDecision": nil,
				"latestOpinionatedReviews": map[string]any{"pageInfo": map[string]any{"hasNextPage": truncated}, "nodes": []any{map[string]any{"state": "APPROVED", "author": map[string]any{"login": "alice"}}}},
				"reviewRequests":           map[string]any{"pageInfo": map[string]any{"hasNextPage": false}, "nodes": []any{}},
			}}}})
		})
		rows := []model.PullRequest{{ID: "N1"}}
		err := client.enrich(context.Background(), rows)
		if truncated {
			if err == nil || rows[0].Review != model.Unknown {
				t.Fatalf("%+v %v", rows, err)
			}
		} else if err != nil || rows[0].Review != model.Approved || rows[0].ReviewSummary.Approved != 1 {
			t.Fatalf("%+v %v", rows, err)
		}
	}
}
