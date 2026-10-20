package github

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestSetAutoMerge(t *testing.T) {
	for _, tc := range []struct {
		name, methods, method  string
		enabled, draft, denied bool
	}{
		{"merge", `"mergeCommitAllowed":true`, "MERGE", true, false, false},
		{"squash only", `"squashMergeAllowed":true`, "SQUASH", true, false, false},
		{"rebase only", `"rebaseMergeAllowed":true`, "REBASE", true, false, false},
		{"disable", `"mergeCommitAllowed":true`, "", false, false, false},
		{"draft", `"mergeCommitAllowed":true`, "", true, true, false},
		{"no methods", `"mergeCommitAllowed":false`, "", true, false, false},
		{"denied", `"mergeCommitAllowed":true`, "MERGE", true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mutations := 0
			client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Query     string
					Variables map[string]any
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatal(err)
				}
				if strings.HasPrefix(req.Query, "query") {
					if req.Variables["owner"] != "acme" || req.Variables["repo"] != "api" || req.Variables["number"] != float64(1) {
						t.Errorf("wrong target: %+v", req.Variables)
					}
					pr := map[string]any{"id": "fresh-id", "state": "OPEN", "isDraft": tc.draft}
					var repo map[string]any
					_ = json.Unmarshal([]byte("{"+tc.methods+"}"), &repo)
					repo["pullRequest"] = pr
					writeJSON(w, map[string]any{"data": map[string]any{"repository": repo}})
					return
				}
				mutations++
				input := req.Variables["input"].(map[string]any)
				if input["pullRequestId"] != "fresh-id" {
					t.Error("used stale node ID")
				}
				if tc.enabled && input["mergeMethod"] != tc.method {
					t.Errorf("wrong method: %+v", input)
				}
				if !tc.enabled && input["mergeMethod"] != nil {
					t.Error("disable has merge method")
				}
				if tc.denied {
					_, _ = w.Write([]byte(`{"errors":[{"message":"secret-private-response"}]}`))
					return
				}
				op := "disablePullRequestAutoMerge"
				var auto any
				if tc.enabled {
					op = "enablePullRequestAutoMerge"
					auto = map[string]any{"enabledAt": "now"}
				}
				if !strings.Contains(req.Query, op+"(") {
					t.Error("wrong mutation")
				}
				writeJSON(w, map[string]any{"data": map[string]any{op: map[string]any{"pullRequest": map[string]any{"id": "fresh-id", "autoMergeRequest": auto, "mergeQueueEntry": nil}}}})
			})
			p := model.PullRequest{ID: "stale-id", Repository: "acme/api", Number: 1, Title: "Keep title", Review: model.Approved, CI: model.Pass}
			got, err := client.SetAutoMerge(context.Background(), p, tc.enabled)
			wantErr := tc.draft || tc.name == "no methods" || tc.denied
			if (err != nil) != wantErr {
				t.Fatalf("err=%v", err)
			}
			if err != nil {
				if strings.Contains(err.Error(), "secret-private-response") {
					t.Fatal("leaked response")
				}
				if !tc.denied && mutations != 0 {
					t.Fatal("invalid PR mutated")
				}
				return
			}
			want := model.Status("OFF")
			if tc.enabled {
				want = "ON"
			}
			if got.AutoMerge != want || got.MergeQueue != "NONE" || got.Title != p.Title || got.Review != p.Review || got.CI != p.CI || mutations != 1 {
				t.Fatalf("got=%+v mutations=%d", got, mutations)
			}
		})
	}
}
