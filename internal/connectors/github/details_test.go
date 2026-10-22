package github

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestDetailsPaginationAndPartialFailure(t *testing.T) {
	client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/graphql":
			writeJSON(w, map[string]any{"data": map[string]any{"nodes": []any{map[string]any{
				"id": "PR7", "reviewRequests": map[string]any{"pageInfo": map[string]any{"hasNextPage": false}, "nodes": []any{}},
				"copilotReviews": map[string]any{"nodes": []any{map[string]any{"id": "new-review", "state": "COMMENTED", "body": "Approval recommended", "author": map[string]any{"login": "Copilot", "__typename": "Bot"}, "comments": map[string]any{"totalCount": 2}}}},
			}}}})
		case r.URL.Path == "/repos/acme/api/pulls/7":
			writeJSON(w, map[string]any{"node_id": "PR7", "head": map[string]any{"sha": "fresh-head"}})
		case strings.HasSuffix(r.URL.Path, "/files"):
			if r.URL.Query().Get("page") == "1" {
				w.Header().Set("Link", "<http://"+r.Host+r.URL.Path+"?page=2>; rel=\"next\"")
				writeJSON(w, []any{map[string]any{"filename": "src/a.go", "status": "modified", "additions": 3}})
			} else {
				writeJSON(w, []any{map[string]any{"filename": "README.md", "status": "added"}})
			}
		case r.URL.Path == "/repos/acme/api/issues/7/comments":
			writeJSON(w, []any{map[string]any{"body": "Looks good", "user": map[string]any{"login": "alice"}}})
		case strings.HasSuffix(r.URL.Path, "/comments") || strings.HasSuffix(r.URL.Path, "/reviews"):
			writeJSON(w, []any{})
		case r.URL.Path == "/repos/acme/api/commits/fresh-head/check-runs":
			w.WriteHeader(http.StatusForbidden)
		default:
			t.Errorf("unexpected request: %s", r.URL)
			w.WriteHeader(404)
		}
	})
	d, err := client.Details(context.Background(), model.PullRequest{Repository: "acme/api", Number: 7, HeadOID: "stale-head"})
	if err != nil {
		t.Fatal(err)
	}
	if d.PullRequest == nil || d.PullRequest.CopilotReview == nil || d.PullRequest.CopilotReview.ID != "new-review" || len(d.PullRequest.RequestedReviewers) != 0 {
		t.Fatalf("review status was not refreshed: %+v", d.PullRequest)
	}
	if len(d.Files) != 2 || d.Files[0].Path != "README.md" || len(d.Comments) != 1 || len(d.Warnings) != 1 {
		t.Fatalf("unexpected details: %+v", d)
	}
}

func TestDetailsStatusFailureKeepsOtherSections(t *testing.T) {
	client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/api/pulls/7":
			writeJSON(w, map[string]any{"node_id": "PR7", "head": map[string]any{"sha": "head"}})
		case "/graphql":
			w.WriteHeader(http.StatusForbidden)
		case "/repos/acme/api/commits/head/check-runs":
			writeJSON(w, map[string]any{"check_runs": []any{}})
		case "/repos/acme/api/pulls/7/files":
			writeJSON(w, []any{map[string]any{"filename": "main.go"}})
		default:
			writeJSON(w, []any{})
		}
	})
	d, err := client.Details(context.Background(), model.PullRequest{Repository: "acme/api", Number: 7})
	if err != nil || d.PullRequest != nil || len(d.Files) != 1 || len(d.Warnings) != 1 || !strings.HasPrefix(d.Warnings[0], "Status:") {
		t.Fatalf("expected files and status warning: %+v, %v", d, err)
	}
}
