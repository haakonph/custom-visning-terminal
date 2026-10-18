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
		case r.URL.Path == "/repos/acme/api/pulls/7":
			writeJSON(w, map[string]any{"head": map[string]any{"sha": "fresh-head"}})
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
	if len(d.Files) != 2 || d.Files[0].Path != "README.md" || len(d.Comments) != 1 || len(d.Warnings) != 1 {
		t.Fatalf("unexpected details: %+v", d)
	}
}
