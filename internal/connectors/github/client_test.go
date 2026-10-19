package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gh "github.com/google/go-github/v93/github"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func fixture(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	base := server.URL + "/"
	client, err := NewWithHTTP("test-token", server.Client(), gh.WithURLs(&base, nil))
	if err != nil {
		t.Fatal(err)
	}
	return client
}
func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
func issue(id string, n int) map[string]any {
	return map[string]any{"node_id": id, "number": n, "title": "Fix login", "html_url": fmt.Sprintf("https://github.com/acme/api/pull/%d", n), "repository_url": "https://api.github.com/repos/acme/api", "user": map[string]any{"login": "alice"}, "pull_request": map[string]any{"url": "api/pr"}, "updated_at": "2026-10-01T12:00:00Z", "created_at": "2026-09-01T12:00:00Z", "labels": []any{map[string]any{"name": "bug"}}}
}
func graph(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Variables struct {
			IDs []string `json:"ids"`
		} `json:"variables"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	nodes := []any{}
	for _, id := range body.Variables.IDs {
		nodes = append(nodes, map[string]any{"id": id, "isDraft": false, "reviewDecision": "APPROVED", "mergeable": "MERGEABLE", "reviewRequests": map[string]any{"nodes": []any{map[string]any{"requestedReviewer": map[string]any{"login": "bob"}}, map[string]any{"requestedReviewer": map[string]any{"slug": "dev", "organization": map[string]any{"login": "acme"}}}}, "pageInfo": map[string]any{"hasNextPage": false}}, "commits": map[string]any{"nodes": []any{map[string]any{"commit": map[string]any{"statusCheckRollup": map[string]any{"state": "SUCCESS"}}}}}})
	}
	writeJSON(w, map[string]any{"data": map[string]any{"nodes": nodes}})
}
func TestSearchFiltersPaginationAndMapping(t *testing.T) {
	for _, view := range []model.View{model.Open, model.Drafts, model.Reviews} {
		t.Run(view.String(), func(t *testing.T) {
			var searches, graphs atomic.Int32
			client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Error("missing token")
				}
				switch r.URL.Path {
				case "/search/issues":
					searches.Add(1)
					if r.URL.Query().Get("q") != query(view, "alice") || r.URL.Query().Get("sort") != "updated" || r.URL.Query().Get("per_page") != "100" {
						t.Errorf("wrong query: %s", r.URL.RawQuery)
					}
					page := r.URL.Query().Get("page")
					n := 1
					if page == "1" {
						w.Header().Set("Link", fmt.Sprintf("<http://%s/search/issues?page=2>; rel=\"next\"", r.Host))
					} else {
						n = 2
					}
					writeJSON(w, map[string]any{"total_count": 2, "items": []any{issue(fmt.Sprintf("N%d", n), n)}})
				case "/graphql":
					graphs.Add(1)
					graph(w, r)
				default:
					t.Errorf("unexpected request %s", r.URL.Path)
					w.WriteHeader(404)
				}
			})
			result, err := client.List(context.Background(), view, "alice")
			if err != nil || !result.Complete || len(result.PRs) != 2 {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			p := result.PRs[0]
			if p.Repository != "acme/api" || p.Author != "alice" || p.Review != model.Approved || p.CI != model.Pass || p.Mergeability != model.Mergeable || p.Labels[0] != "bug" || p.RequestedReviewers[0] != "bob" || p.RequestedTeams[0] != "acme/dev" || p.CreatedAt.IsZero() {
				t.Fatalf("bad mapping: %+v", p)
			}
			if searches.Load() != 2 || graphs.Load() != 1 {
				t.Fatalf("unexpected calls: search=%d graph=%d", searches.Load(), graphs.Load())
			}
		})
	}
	if q := query(model.Reviews, "alice"); strings.Contains(q, "review-requested:alice") && !strings.Contains(q, "user-review-requested:alice") {
		t.Fatal("mixed direct/team requests")
	}
}
func TestTeamsPaginationMultipleOrganizationsAndDedup(t *testing.T) {
	var searches atomic.Int32
	client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user/teams":
			org := "acme"
			if r.URL.Query().Get("page") == "1" {
				w.Header().Set("Link", fmt.Sprintf("<http://%s/user/teams?page=2>; rel=\"next\"", r.Host))
			} else {
				org = "other"
			}
			writeJSON(w, []any{map[string]any{"slug": "dev", "organization": map[string]any{"login": org}}})
		case "/search/issues":
			searches.Add(1)
			q := r.URL.Query().Get("q")
			if q != "is:open is:pr team-review-requested:acme/dev -author:alice" && q != "is:open is:pr team-review-requested:other/dev -author:alice" {
				t.Errorf("wrong team query: %s", q)
			}
			other := issue("N1", 1)
			other["user"] = map[string]any{"login": "bob"}
			writeJSON(w, map[string]any{"total_count": 2, "items": []any{other, issue("OWN", 2)}})
		case "/graphql":
			graph(w, r)
		default:
			w.WriteHeader(404)
		}
	})
	result, err := client.List(context.Background(), model.TeamReviews, "alice")
	if err != nil || !result.Complete || len(result.PRs) != 1 || searches.Load() != 2 {
		t.Fatalf("%+v %v", result, err)
	}
}
func TestTeamAccessDeniedAndSearchPartial(t *testing.T) {
	t.Run("membership denied", func(t *testing.T) {
		client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(403)
			fmt.Fprint(w, `{"message":"private secret"}`)
		})
		result, err := client.List(context.Background(), model.TeamReviews, "alice")
		if err != nil || result.Complete || len(result.Warnings) == 0 {
			t.Fatalf("%+v %v", result, err)
		}
		if strings.Contains(strings.Join(result.Warnings, " "), "private secret") {
			t.Fatal("leaked response")
		}
	})
	t.Run("search cap", func(t *testing.T) {
		client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/graphql" {
				graph(w, r)
				return
			}
			writeJSON(w, map[string]any{"total_count": 1001, "incomplete_results": true, "items": []any{issue("N1", 1)}})
		})
		result, err := client.List(context.Background(), model.Open, "alice")
		if err != nil || result.Complete || len(result.PRs) != 1 || len(result.Warnings) == 0 {
			t.Fatalf("%+v %v", result, err)
		}
	})
	t.Run("page failure retains results", func(t *testing.T) {
		client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/graphql" {
				graph(w, r)
				return
			}
			if r.URL.Query().Get("page") == "2" {
				w.WriteHeader(422)
				return
			}
			w.Header().Set("Link", fmt.Sprintf("<http://%s/search/issues?page=2>; rel=\"next\"", r.Host))
			writeJSON(w, map[string]any{"total_count": 2, "items": []any{issue("N1", 1)}})
		})
		result, err := client.List(context.Background(), model.Open, "alice")
		if err == nil || result.Complete || len(result.PRs) != 1 {
			t.Fatalf("%+v %v", result, err)
		}
	})
}
func TestGraphQLPartialAndUnknown(t *testing.T) {
	client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/graphql" {
			writeJSON(w, map[string]any{"data": map[string]any{"nodes": []any{nil}}, "errors": []any{map[string]any{"message": "sensitive detail"}}})
			return
		}
		writeJSON(w, map[string]any{"total_count": 1, "items": []any{issue("N1", 1)}})
	})
	result, err := client.List(context.Background(), model.Open, "alice")
	if err != nil || result.Complete || result.PRs[0].Review != model.Unknown || result.PRs[0].CI != model.Unknown {
		t.Fatalf("%+v %v", result, err)
	}
	if strings.Contains(strings.Join(result.Warnings, " "), "sensitive") {
		t.Fatal("leaked GraphQL error")
	}
}
func TestUserAndAuthErrors(t *testing.T) {
	for _, code := range []int{200, 401, 403} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			var calls atomic.Int32
			client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(code)
				fmt.Fprint(w, `{"login":"alice","message":"secret-token"}`)
			})
			user, err := client.User(context.Background())
			if code == 200 {
				if err != nil || user != "alice" {
					t.Fatalf("%s %v", user, err)
				}
			} else if err == nil || strings.Contains(err.Error(), "secret-token") {
				t.Fatalf("unsafe error: %v", err)
			}
			if calls.Load() != 1 {
				t.Fatal("authentication error retried")
			}
		})
	}
}
func TestCancellationAndTimeout(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(fmt.Sprint(timeout), func(t *testing.T) {
			client := fixture(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
			var ctx context.Context
			var cancel context.CancelFunc
			if timeout {
				ctx, cancel = context.WithTimeout(context.Background(), 15*time.Millisecond)
			} else {
				ctx, cancel = context.WithCancel(context.Background())
				cancel()
			}
			defer cancel()
			start := time.Now()
			_, err := client.User(ctx)
			if err == nil || time.Since(start) > time.Second {
				t.Fatalf("cancellation failed: %v", err)
			}
			if timeout && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("deadline: %v", err)
			}
		})
	}
}
func TestRetryAfterAndPermanentErrors(t *testing.T) {
	for _, code := range []int{429, 503, 401, 422} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			var calls atomic.Int32
			client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				if n == 1 {
					w.Header().Set("Retry-After", "0")
					w.WriteHeader(code)
					fmt.Fprint(w, `{"message":"error"}`)
					return
				}
				writeJSON(w, map[string]any{"login": "alice"})
			})
			user, err := client.User(context.Background())
			if code == 429 || code == 503 {
				if err != nil || user != "alice" || calls.Load() != 2 {
					t.Fatalf("retry %d %s %v", calls.Load(), user, err)
				}
			} else if err == nil || calls.Load() != 1 {
				t.Fatal("retried permanent error")
			}
		})
	}
	now := time.Now().UTC().Truncate(time.Second)
	h := http.Header{"Retry-After": []string{now.Add(2 * time.Second).Format(http.TimeFormat)}}
	if d, ok := retryDelay(h, now); !ok || d != 2*time.Second {
		t.Fatalf("date retry: %v %v", d, ok)
	}
	h = http.Header{"X-Ratelimit-Remaining": []string{"0"}, "X-Ratelimit-Reset": []string{fmt.Sprint(now.Add(time.Minute).Unix())}}
	if d, ok := retryDelay(h, now); !ok || d != time.Minute {
		t.Fatalf("rate reset: %v %v", d, ok)
	}
}

func TestBatchEnrichmentAndTenPageCap(t *testing.T) {
	t.Run("batch", func(t *testing.T) {
		var graphs atomic.Int32
		client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/graphql" {
				graphs.Add(1)
				graph(w, r)
				return
			}
			items := []any{}
			for i := 0; i < 81; i++ {
				items = append(items, issue(fmt.Sprintf("N%d", i), i+1))
			}
			writeJSON(w, map[string]any{"total_count": 81, "items": items})
		})
		result, err := client.List(context.Background(), model.Open, "alice")
		if err != nil || !result.Complete || len(result.PRs) != 81 || graphs.Load() != 3 {
			t.Fatalf("rows=%d batches=%d err=%v", len(result.PRs), graphs.Load(), err)
		}
	})
	t.Run("ten pages", func(t *testing.T) {
		var searches atomic.Int32
		client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/graphql" {
				graph(w, r)
				return
			}
			searches.Add(1)
			w.Header().Set("Link", fmt.Sprintf("<http://%s/search/issues?page=99>; rel=\"next\"", r.Host))
			writeJSON(w, map[string]any{"total_count": 10000, "items": []any{issue("N1", 1)}})
		})
		result, err := client.List(context.Background(), model.Open, "alice")
		if err != nil || result.Complete || searches.Load() != 10 || len(result.PRs) != 1 {
			t.Fatalf("%+v calls=%d %v", result, searches.Load(), err)
		}
	})
}
func TestStatusMapping(t *testing.T) {
	for _, test := range []struct {
		decision, ci, merge         string
		review, check, mergeability model.Status
	}{
		{"APPROVED", "SUCCESS", "MERGEABLE", model.Approved, model.Pass, model.Mergeable},
		{"CHANGES_REQUESTED", "FAILURE", "CONFLICTING", model.ChangesRequested, model.Fail, model.Conflicting},
		{"REVIEW_REQUIRED", "PENDING", "UNKNOWN", model.Pending, model.Pending, model.Unknown},
		{"", "", "", model.Unknown, model.Unknown, model.Unknown},
	} {
		raw := fmt.Sprintf(`{"id":"N1","reviewDecision":%q,"mergeable":%q,"commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":%q}}}]}}`, test.decision, test.merge, test.ci)
		var d details
		if err := json.Unmarshal([]byte(raw), &d); err != nil {
			t.Fatal(err)
		}
		p := model.PullRequest{Review: model.Unknown, CI: model.Unknown, Mergeability: model.Unknown}
		applyDetails(&p, &d)
		if p.Review != test.review || p.CI != test.check || p.Mergeability != test.mergeability {
			t.Fatalf("%+v", p)
		}
	}
}
func TestCancellationDuringRetryWait(t *testing.T) {
	client := fixture(t, func(w http.ResponseWriter, r *http.Request) { w.Header().Set("Retry-After", "5"); w.WriteHeader(503) })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := client.User(ctx)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
		t.Fatalf("retry wait ignored cancellation: %v", err)
	}
}
