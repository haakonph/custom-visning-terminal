package github

import (
	"encoding/json"
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
			raw, err := json.Marshal(map[string]any{"copilotReviews": map[string]any{"nodes": []any{map[string]any{"body": tc.body, "state": tc.state, "comments": map[string]any{"totalCount": 3}}}}})
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
