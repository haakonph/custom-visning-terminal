package github

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestOpenCommentsPagination(t *testing.T) {
	client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query     string
			Variables map[string]any
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Variables["after"] != "next" || req.Variables["id"] != "N1" || !strings.Contains(req.Query, "comments(first:1)") {
			t.Errorf("unexpected query: %+v", req)
		}
		writeJSON(w, map[string]any{"data": map[string]any{"node": map[string]any{"reviewThreads": map[string]any{
			"pageInfo": map[string]any{"hasNextPage": false},
			"nodes":    []any{map[string]any{"isResolved": false, "comments": map[string]any{"nodes": []any{map[string]any{"author": nil}}}}},
		}}}})
	})
	var page reviewThreads
	err := json.Unmarshal([]byte(`{"pageInfo":{"hasNextPage":true,"endCursor":"next"},"nodes":[
 {"isResolved":false,"comments":{"nodes":[{"author":{"__typename":"User"}}]}},
 {"isResolved":false,"comments":{"nodes":[{"author":{"__typename":"Bot"}}]}},
 {"isResolved":true}]}`), &page)
	if err != nil {
		t.Fatal(err)
	}
	p := model.PullRequest{ID: "N1"}
	if err := client.completeOpenComments(context.Background(), &p, &page); err != nil {
		t.Fatal(err)
	}
	if p.OpenComments == nil || *p.OpenComments != 2 {
		t.Fatalf("count = %v", p.OpenComments)
	}
}

func TestOpenCommentsUnavailableAndEmpty(t *testing.T) {
	client := fixture(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) })
	for _, tc := range []struct {
		name, raw        string
		wantError, known bool
	}{
		{"missing", `null`, false, false},
		{"empty", `{"pageInfo":{"hasNextPage":false},"nodes":[]}`, false, true},
		{"missing nodes", `{"pageInfo":{"hasNextPage":false}}`, true, false},
		{"partial node", `{"pageInfo":{"hasNextPage":false},"nodes":[null]}`, true, false},
		{"missing resolution", `{"pageInfo":{"hasNextPage":false},"nodes":[{}]}`, true, false},
		{"missing cursor", `{"pageInfo":{"hasNextPage":true},"nodes":[]}`, true, false},
		{"failed page", `{"pageInfo":{"hasNextPage":true,"endCursor":"next"},"nodes":[]}`, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var page *reviewThreads
			if err := json.Unmarshal([]byte(tc.raw), &page); err != nil {
				t.Fatal(err)
			}
			stale := 7
			p := model.PullRequest{ID: "N1", OpenComments: &stale}
			err := client.completeOpenComments(context.Background(), &p, page)
			if (err != nil) != tc.wantError || (p.OpenComments != nil) != tc.known {
				t.Fatalf("count=%v err=%v", p.OpenComments, err)
			}
			if tc.known && *p.OpenComments != 0 {
				t.Fatal(*p.OpenComments)
			}
		})
	}
}
