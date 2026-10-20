package github

import (
	"context"
	"net/http"
	"testing"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestClosedRequiresExplicitState(t *testing.T) {
	for _, tc := range []struct {
		name, body      string
		code            int
		closed, failure bool
	}{
		{"open", `{"state":"open"}`, 200, false, false},
		{"closed", `{"state":"closed"}`, 200, true, false},
		{"merged", `{"state":"closed","merged":true}`, 200, true, false},
		{"missing", `{}`, 200, false, true},
		{"unknown", `{"state":"unknown"}`, 200, false, true},
		{"not found", `{}`, 404, false, true},
		{"access denied", `{}`, 403, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/repos/acme/repo/pulls/42" {
					t.Error(r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.code)
				_, _ = w.Write([]byte(tc.body))
			})
			got, err := client.Closed(context.Background(), model.PullRequest{Repository: "acme/repo", Number: 42})
			if got != tc.closed || (err != nil) != tc.failure {
				t.Fatalf("got %v, %v", got, err)
			}
		})
	}
}
