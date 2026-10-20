package github

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestCopilotRecommendation(t *testing.T) {
	for _, tc := range []struct{ body, state, want string }{
		{"## 🟢 Approval recommended\nSummary", "COMMENTED", "Approval"},
		{"## 🔴 Changes recommended", "COMMENTED", "Changes"},
		{"## 🟡 Further review recommended", "COMMENTED", "Review"},
		{"Reviewed 3 files", "COMMENTED", "Comments"},
		{"Approval recommended", "DISMISSED", "Dismissed"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{"copilotReviews": map[string]any{"nodes": []any{map[string]any{"author": map[string]any{"login": "Copilot", "__typename": "Bot"}, "body": tc.body, "state": tc.state, "comments": map[string]any{"totalCount": 3}}}}})
			if err != nil {
				t.Fatal(err)
			}
			var d details
			if err := json.Unmarshal(raw, &d); err != nil {
				t.Fatal(err)
			}
			p := model.PullRequest{Review: model.Pending}
			applyCopilotReview(&p, &d)
			if p.CopilotReview == nil || p.CopilotReview.Recommendation != tc.want || p.CopilotReview.Comments != 3 || p.Review != model.Pending {
				t.Fatalf("unexpected review: %+v", p)
			}
			applyCopilotReview(&p, &details{})
			if p.CopilotReview != nil {
				t.Fatal("stale Copilot review retained")
			}
		})
	}
}

func TestLatestCopilotIdentityAndOrdering(t *testing.T) {
	for _, login := range []string{"Copilot", "copilot-pull-request-reviewer", "copilot-pull-request-reviewer[bot]"} {
		var d details
		raw := `{"copilotReviews":{"nodes":[{"author":{"login":"Copilot","__typename":"Bot"},"body":"Changes recommended","comments":{"totalCount":7}},{"author":{"login":"` + login + `","__typename":"Bot"},"body":"Approval recommended","comments":{"totalCount":2}},{"author":{"login":"alice","__typename":"User"},"body":"Changes recommended"},null,{"author":null},{"author":{"login":"Copilot","__typename":"User"},"body":"Changes recommended"}]}}`
		if err := json.Unmarshal([]byte(raw), &d); err != nil {
			t.Fatal(err)
		}
		var p model.PullRequest
		applyCopilotReview(&p, &d)
		if p.CopilotReview == nil || p.CopilotReview.Recommendation != "Approval" || p.CopilotReview.Comments != 2 {
			t.Fatalf("%s: %+v", login, p.CopilotReview)
		}
	}
}

func TestCopilotReviewPagination(t *testing.T) {
	calls := 0
	client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req struct {
			Variables struct {
				ID     string `json:"id"`
				Before string `json:"before"`
			} `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if req.Variables.ID != "PR1" || req.Variables.Before != "older" {
			t.Errorf("unexpected variables: %+v", req.Variables)
		}
		var payload any
		if err := json.Unmarshal([]byte(`{"data":{"node":{"copilotReviews":{"pageInfo":{"hasPreviousPage":true,"startCursor":"earlier"},"nodes":[{"author":{"login":"Copilot","__typename":"Bot"},"body":"Approval recommended","comments":{"totalCount":4}}]}}}}`), &payload); err != nil {
			t.Fatal(err)
		}
		writeJSON(w, payload)
	})
	var d details
	d.CopilotReviews.PageInfo.HasPreviousPage = true
	d.CopilotReviews.PageInfo.StartCursor = "older"
	if err := client.completeCopilotReviews(context.Background(), "PR1", &d); err != nil {
		t.Fatal(err)
	}
	var p model.PullRequest
	applyCopilotReview(&p, &d)
	if calls != 1 || p.CopilotReview == nil || p.CopilotReview.Comments != 4 {
		t.Fatalf("calls=%d review=%+v", calls, p.CopilotReview)
	}
}
func TestCopilotReviewRejectsRepeatedCursor(t *testing.T) {
	client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"data": map[string]any{"node": map[string]any{"copilotReviews": map[string]any{"pageInfo": map[string]any{"hasPreviousPage": true, "startCursor": "same"}}}}})
	})
	var d details
	d.CopilotReviews.PageInfo.HasPreviousPage = true
	d.CopilotReviews.PageInfo.StartCursor = "same"
	if err := client.completeCopilotReviews(context.Background(), "PR1", &d); err == nil {
		t.Fatal("expected pagination error")
	}
}

func TestCopilotReviewIdentityAndPendingDraft(t *testing.T) {
	var d details
	if err := json.Unmarshal([]byte(`{"copilotReviews":{"nodes":[{"id":"submitted","author":{"login":"Copilot","__typename":"Bot"},"state":"COMMENTED"},{"id":"draft","author":{"login":"Copilot","__typename":"Bot"},"state":"PENDING"}]}}`), &d); err != nil {
		t.Fatal(err)
	}
	var p model.PullRequest
	applyCopilotReview(&p, &d)
	if p.CopilotReview == nil || p.CopilotReview.ID != "submitted" {
		t.Fatalf("expected the completed review identity, got %+v", p.CopilotReview)
	}
}
