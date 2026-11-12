package github

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestLatestReviewSinceRequiresCurrentUsersSubmittedReview(t *testing.T) {
	since := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, body string
		want       model.ReviewOutcome
	}{
		{"approval", `[{"user":{"login":"Alice"},"state":"APPROVED","submitted_at":"2026-11-12T10:01:00Z"}]`, model.ReviewApproval},
		{"changes", `[{"user":{"login":"alice"},"state":"CHANGES_REQUESTED","submitted_at":"2026-11-12T10:01:00Z"}]`, ""},
		{"comments", `[{"user":{"login":"alice"},"state":"COMMENTED","submitted_at":"2026-11-12T10:01:00Z"}]`, model.ReviewComment},
		{"other reviewer", `[{"user":{"login":"bob"},"state":"APPROVED","submitted_at":"2026-11-12T10:01:00Z"}]`, ""},
		{"old review", `[{"user":{"login":"alice"},"state":"APPROVED","submitted_at":"2026-11-12T09:59:00Z"}]`, ""},
		{"draft", `[{"user":{"login":"alice"},"state":"PENDING"}]`, ""},
		{"dismissed", `[{"user":{"login":"alice"},"state":"DISMISSED","submitted_at":"2026-11-12T10:01:00Z"}]`, ""},
		{"missing data", `[null,{"state":"APPROVED"}]`, ""},
		{"no reviews", `[]`, ""},
		{"comment replaces approval", `[{"user":{"login":"alice"},"state":"APPROVED","submitted_at":"2026-11-12T10:01:00Z"},{"user":{"login":"alice"},"state":"COMMENTED","submitted_at":"2026-11-12T10:02:00Z"}]`, model.ReviewComment},
		{"approval replaces comment", `[{"user":{"login":"alice"},"state":"COMMENTED","submitted_at":"2026-11-12T10:01:00Z"},{"user":{"login":"alice"},"state":"APPROVED","submitted_at":"2026-11-12T10:02:00Z"}]`, model.ReviewApproval},
		{"changes replace approval", `[{"user":{"login":"alice"},"state":"APPROVED","submitted_at":"2026-11-12T10:01:00Z"},{"user":{"login":"alice"},"state":"CHANGES_REQUESTED","submitted_at":"2026-11-12T10:02:00Z"}]`, ""},
		{"dismissal replaces approval", `[{"user":{"login":"alice"},"state":"APPROVED","submitted_at":"2026-11-12T10:01:00Z"},{"user":{"login":"alice"},"state":"DISMISSED","submitted_at":"2026-11-12T10:02:00Z"}]`, ""},
		{"latest timestamp wins", `[{"user":{"login":"alice"},"state":"APPROVED","submitted_at":"2026-11-12T10:02:00Z"},{"user":{"login":"alice"},"state":"COMMENTED","submitted_at":"2026-11-12T10:01:00Z"}]`, model.ReviewApproval},
		{"same second uses review ID", `[{"id":2,"user":{"login":"alice"},"state":"COMMENTED","submitted_at":"2026-11-12T10:01:00Z"},{"id":1,"user":{"login":"alice"},"state":"APPROVED","submitted_at":"2026-11-12T10:01:00Z"}]`, model.ReviewComment},
		{"later draft ignored", `[{"user":{"login":"alice"},"state":"COMMENTED","submitted_at":"2026-11-12T10:01:00Z"},{"user":{"login":"alice"},"state":"PENDING","submitted_at":"2026-11-12T10:02:00Z"}]`, model.ReviewComment},
		{"later other reviewer ignored", `[{"user":{"login":"alice"},"state":"COMMENTED","submitted_at":"2026-11-12T10:01:00Z"},{"user":{"login":"bob"},"state":"APPROVED","submitted_at":"2026-11-12T10:02:00Z"}]`, model.ReviewComment},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/repos/acme/api/pulls/42/reviews" || r.URL.Query().Get("per_page") != "100" {
					t.Error(r.Method, r.URL)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			})
			got, err := client.LatestReviewSince(context.Background(), model.PullRequest{Repository: "acme/api", Number: 42}, "alice", since)
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestLatestReviewSincePaginatesAndFailsClosed(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				if calls == 1 {
					w.Header().Set("Link", fmt.Sprintf(`<%s%s?page=2>; rel="next"`, "http://"+r.Host, r.URL.Path))
					_, _ = w.Write([]byte(`[{"user":{"login":"alice"},"state":"APPROVED","submitted_at":"2026-11-12T10:01:00Z"}]`))
					return
				}
				if r.URL.Query().Get("page") != "2" {
					t.Error("missing pagination", r.URL)
				}
				w.WriteHeader(status)
				if status == http.StatusOK {
					_, _ = w.Write([]byte(`[{"user":{"login":"alice"},"state":"COMMENTED","submitted_at":"2026-11-12T10:02:00Z"}]`))
				} else {
					_, _ = w.Write([]byte(`{"message":"private upstream details"}`))
				}
			})
			got, err := client.LatestReviewSince(context.Background(), model.PullRequest{Repository: "acme/api", Number: 42}, "alice", time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC))
			var want model.ReviewOutcome
			if status == http.StatusOK {
				want = model.ReviewComment
			}
			if calls != 2 || got != want || (err != nil) != (status != http.StatusOK) {
				t.Fatal(calls, got, err)
			}
		})
	}
}
