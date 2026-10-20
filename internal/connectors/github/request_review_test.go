package github

import (
	"context"
	"encoding/json"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"net/http"
	"testing"
)

func TestRequestCopilotReview(t *testing.T) {
	for _, status := range []int{201, 403, 422} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			c := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "POST" || r.URL.Path != "/repos/acme/api/pulls/7/requested_reviewers" {
					t.Errorf("wrong request: %s %s", r.Method, r.URL.Path)
				}
				var body struct {
					Reviewers []string `json:"reviewers"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if len(body.Reviewers) != 1 || body.Reviewers[0] != "copilot-pull-request-reviewer[bot]" {
					t.Errorf("wrong reviewers: %v", body.Reviewers)
				}
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{}`))
			})
			err := c.RequestCopilotReview(context.Background(), model.PullRequest{Repository: "acme/api", Number: 7, CopilotReview: &model.CopilotReview{Recommendation: "COMMENTED"}})
			if (err == nil) != (status == 201) || calls != 1 {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
			if c.RequestCopilotReview(context.Background(), model.PullRequest{Repository: "bad", Number: 7}) == nil || calls != 1 {
				t.Fatal("invalid target sent")
			}
		})
	}
}
