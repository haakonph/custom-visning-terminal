package github

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestReviewedSinceRequiresCurrentUsersSubmittedReview(t *testing.T) {
	since := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, body string
		want       bool
	}{
		{"approval", `[{"user":{"login":"Alice"},"state":"APPROVED","submitted_at":"2026-11-12T10:01:00Z"}]`, true},
		{"changes", `[{"user":{"login":"alice"},"state":"CHANGES_REQUESTED","submitted_at":"2026-11-12T10:01:00Z"}]`, true},
		{"comments", `[{"user":{"login":"alice"},"state":"COMMENTED","submitted_at":"2026-11-12T10:01:00Z"}]`, true},
		{"other reviewer", `[{"user":{"login":"bob"},"state":"APPROVED","submitted_at":"2026-11-12T10:01:00Z"}]`, false},
		{"old review", `[{"user":{"login":"alice"},"state":"APPROVED","submitted_at":"2026-11-12T09:59:00Z"}]`, false},
		{"draft", `[{"user":{"login":"alice"},"state":"PENDING"}]`, false},
		{"dismissed", `[{"user":{"login":"alice"},"state":"DISMISSED","submitted_at":"2026-11-12T10:01:00Z"}]`, false},
		{"missing data", `[null,{"state":"APPROVED"}]`, false},
		{"no reviews", `[]`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/repos/acme/api/pulls/42/reviews" || r.URL.Query().Get("per_page") != "100" {
					t.Error(r.Method, r.URL)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			})
			got, err := client.ReviewedSince(context.Background(), model.PullRequest{Repository: "acme/api", Number: 42}, "alice", since)
			if err != nil || got != tc.want {
				t.Fatalf("got %v, %v; want %v", got, err, tc.want)
			}
		})
	}
}

func TestReviewedSincePaginatesAndFailsClosed(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				if calls == 1 {
					w.Header().Set("Link", fmt.Sprintf(`<%s%s?page=2>; rel="next"`, "http://"+r.Host, r.URL.Path))
					_, _ = w.Write([]byte(`[]`))
					return
				}
				if r.URL.Query().Get("page") != "2" {
					t.Error("missing pagination", r.URL)
				}
				w.WriteHeader(status)
				if status == http.StatusOK {
					_, _ = w.Write([]byte(`[{"user":{"login":"alice"},"state":"APPROVED","submitted_at":"2026-11-12T10:01:00Z"}]`))
				} else {
					_, _ = w.Write([]byte(`{"message":"private upstream details"}`))
				}
			})
			got, err := client.ReviewedSince(context.Background(), model.PullRequest{Repository: "acme/api", Number: 42}, "alice", time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC))
			if calls != 2 || got != (status == http.StatusOK) || (err != nil) != (status != http.StatusOK) {
				t.Fatal(calls, got, err)
			}
		})
	}
}
