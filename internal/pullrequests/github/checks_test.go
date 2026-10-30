package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestCheckStatus(t *testing.T) {
	for _, tc := range []struct {
		name   string
		checks string
		want   model.Status
	}{
		{"required failure", `{"name":"test","status":"COMPLETED","conclusion":"FAILURE","isRequired":true}`, model.Fail},
		{"optional failure", `{"name":"lint","status":"COMPLETED","conclusion":"FAILURE","isRequired":false}`, model.Warning},
		{"required texts", `{"name":"build / tekst-sync (nb)","status":"COMPLETED","conclusion":"FAILURE","isRequired":true}`, model.Texts},
		{"optional texts", `{"name":"TEKST-SYNC","status":"COMPLETED","conclusion":"FAILURE","isRequired":false}`, model.Texts},
		{"required status context", `{"context":"build","state":"ERROR","isRequired":true}`, model.Fail},
		{"optional status context", `{"context":"build","state":"FAILURE","isRequired":false}`, model.Warning},
		{"text status context", `{"context":"workflow/tekst-sync","state":"ERROR","isRequired":true}`, model.Texts},
		{"required beats texts", `{"context":"tekst-sync","state":"FAILURE","isRequired":true},{"context":"build","state":"FAILURE","isRequired":true}`, model.Fail},
		{"texts beat warning", `{"context":"tekst-sync","state":"FAILURE","isRequired":true},{"context":"lint","state":"FAILURE","isRequired":false}`, model.Texts},
		{"optional texts beat warning regardless of order", `{"context":"lint","state":"FAILURE","isRequired":false},{"context":"tekst-sync","state":"FAILURE","isRequired":false}`, model.Texts},
		{"required beats texts and warning", `{"context":"lint","state":"FAILURE","isRequired":false},{"context":"tekst-sync","state":"FAILURE","isRequired":true},{"context":"test","state":"FAILURE","isRequired":true}`, model.Fail},
		{"required beats warning", `{"context":"lint","state":"FAILURE","isRequired":false},{"context":"test","state":"FAILURE","isRequired":true}`, model.Fail},
		{"pending text check", `{"name":"tekst-sync","status":"IN_PROGRESS","isRequired":true}`, model.Pending},
		{"passing text check", `{"name":"tekst-sync","status":"COMPLETED","conclusion":"SUCCESS","isRequired":true}`, model.Pass},
		{"neutral and skipped", `{"name":"lint","status":"COMPLETED","conclusion":"NEUTRAL"},{"name":"test","status":"COMPLETED","conclusion":"SKIPPED"}`, model.Pass},
		{"timed out", `{"name":"test","status":"COMPLETED","conclusion":"TIMED_OUT","isRequired":true}`, model.Fail},
		{"cancelled optional", `{"name":"test","status":"COMPLETED","conclusion":"CANCELLED","isRequired":false}`, model.Warning},
		{"unknown requirement", `{"name":"test","status":"COMPLETED","conclusion":"FAILURE"}`, model.Unknown},
		{"unknown conclusion", `{"name":"test","status":"COMPLETED","conclusion":"NEW_STATE","isRequired":true}`, model.Unknown},
		{"pending context", `{"context":"test","state":"PENDING","isRequired":true}`, model.Pending},
		{"expected required context", `{"context":"test","state":"EXPECTED","isRequired":true}`, model.Missing},
		{"expected optional context", `{"context":"test","state":"EXPECTED","isRequired":false}`, model.Pending},
		{"queued required check", `{"name":"test","status":"QUEUED","isRequired":true}`, model.Missing},
		{"waiting required check", `{"name":"test","status":"WAITING","isRequired":true}`, model.Missing},
		{"requested required check", `{"name":"test","status":"REQUESTED","isRequired":true}`, model.Missing},
		{"pending required check run", `{"name":"test","status":"PENDING","isRequired":true}`, model.Missing},
		{"queued optional check", `{"name":"test","status":"QUEUED","isRequired":false}`, model.Pending},
		{"required error beats missing", `{"name":"test","status":"COMPLETED","conclusion":"FAILURE","isRequired":true},{"name":"build","status":"QUEUED","isRequired":true}`, model.Fail},
		{"missing beats pending", `{"name":"test","status":"QUEUED","isRequired":true},{"name":"build","status":"IN_PROGRESS","isRequired":true}`, model.Missing},
		{"missing beats warning", `{"context":"test","state":"EXPECTED","isRequired":true},{"context":"lint","state":"FAILURE","isRequired":false}`, model.Missing},
		{"pending beats warning", `{"name":"test","status":"IN_PROGRESS","isRequired":true},{"context":"lint","state":"FAILURE","isRequired":false}`, model.Pending},
		{"pending beats texts", `{"name":"test","status":"IN_PROGRESS","isRequired":true},{"context":"tekst-sync","state":"FAILURE","isRequired":true}`, model.Pending},
		{"missing beats texts", `{"name":"test","status":"QUEUED","isRequired":true},{"context":"tekst-sync","state":"FAILURE","isRequired":true}`, model.Missing},
		{"unrecognized check status", `{"name":"test","status":"NEW_STATE","isRequired":true}`, model.Unknown},
		{"empty", ``, model.Unknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var rollup checkRollup
			if err := json.Unmarshal([]byte(`{"state":"FAILURE","contexts":{"nodes":[`+tc.checks+`],"pageInfo":{"hasNextPage":false}}}`), &rollup); err != nil {
				t.Fatal(err)
			}
			if got := checkStatus(&rollup); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestCheckStatusMissingRequirements(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rollup   string
		required []string
		want     model.Status
	}{
		{"no checks or requirements", `null`, nil, model.Unknown},
		{"no checks on required commit", `null`, []string{"test"}, model.Missing},
		{"empty checks", `{"contexts":{"nodes":[]}}`, []string{"test"}, model.Missing},
		{"missing alongside success", `{"contexts":{"nodes":[{"name":"build","status":"COMPLETED","conclusion":"SUCCESS"}]}}`, []string{"test", "build"}, model.Missing},
		{"all requirements present", `{"contexts":{"nodes":[{"name":"build","status":"COMPLETED","conclusion":"SUCCESS"}]}}`, []string{"build", "build"}, model.Pass},
		{"required status context present", `{"contexts":{"nodes":[{"context":"test","state":"SUCCESS"}]}}`, []string{"test"}, model.Pass},
		{"required failure wins", `{"contexts":{"nodes":[{"context":"test","state":"FAILURE","isRequired":true}]}}`, []string{"test", "build"}, model.Fail},
		{"rules identify required failure", `{"contexts":{"nodes":[{"context":"test","state":"FAILURE"}]}}`, []string{"test"}, model.Fail},
		{"missing beats running", `{"contexts":{"nodes":[{"name":"build","status":"IN_PROGRESS","isRequired":true}]}}`, []string{"test", "build"}, model.Missing},
		{"missing beats optional failure", `{"contexts":{"nodes":[{"context":"lint","state":"FAILURE","isRequired":false}]}}`, []string{"test"}, model.Missing},
		{"incomplete checks cannot prove missing", `{"contexts":{"nodes":[],"pageInfo":{"hasNextPage":true}}}`, []string{"test"}, model.Unknown},
		{"aggregate expected", `{"state":"EXPECTED"}`, nil, model.Missing},
		{"aggregate pending", `{"state":"PENDING"}`, nil, model.Pending},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var rollup *checkRollup
			if err := json.Unmarshal([]byte(tc.rollup), &rollup); err != nil {
				t.Fatal(err)
			}
			if got := checkStatus(rollup, tc.required...); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func checkDetailsJSON(nodes, pageInfo string) string {
	return `{"commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"FAILURE","contexts":{"nodes":[` + nodes + `],"pageInfo":` + pageInfo + `}}}}]}}`
}

func TestChecksUseEachPullRequestID(t *testing.T) {
	client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		for i, id := range []string{"N1", "N2"} {
			start := strings.Index(req.Query, fmt.Sprintf("ci%d:node(id:%q)", i, id))
			if start < 0 {
				t.Fatalf("missing alias for %s", id)
			}
			part := req.Query[start:]
			if next := strings.Index(part[1:], "ci1:node"); next >= 0 {
				part = part[:next+1]
			}
			if strings.Count(part, fmt.Sprintf("isRequired(pullRequestId:%q)", id)) != 2 {
				t.Errorf("checks do not use PR ID %s", id)
			}
		}
		_, _ = fmt.Fprintf(w, `{"data":{"nodes":[{"id":"N1"},{"id":"N2"}],"ci0":%s,"ci1":%s}}`,
			checkDetailsJSON(`{"context":"test","state":"FAILURE","isRequired":true}`, `{"hasNextPage":false}`),
			checkDetailsJSON(`{"context":"test","state":"FAILURE","isRequired":false}`, `{"hasNextPage":false}`))
	})
	rows := []model.PullRequest{{ID: "N1"}, {ID: "N2"}}
	if err := client.enrich(context.Background(), rows); err != nil {
		t.Fatal(err)
	}
	if rows[0].CI != model.Fail || rows[1].CI != model.Warning {
		t.Fatalf("incorrect per-PR requirements: %+v", rows)
	}
}

func TestChecksPagination(t *testing.T) {
	for _, broken := range []bool{false, true} {
		t.Run(fmt.Sprintf("broken=%v", broken), func(t *testing.T) {
			calls := 0
			client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				var req struct {
					Query string `json:"query"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatal(err)
				}
				if calls == 1 {
					_, _ = fmt.Fprintf(w, `{"data":{"nodes":[{"id":"N1"}],"ci0":%s}}`, checkDetailsJSON(`{"context":"tekst-sync","state":"FAILURE","isRequired":true}`, `{"hasNextPage":true,"endCursor":"next"}`))
					return
				}
				if !strings.Contains(req.Query, `after:"next"`) {
					t.Error("missing next-page cursor")
				}
				if broken {
					_, _ = w.Write([]byte(`{"data":{"ci0":null},"errors":[{"message":"unavailable"}]}`))
					return
				}
				_, _ = fmt.Fprintf(w, `{"data":{"ci0":%s}}`, checkDetailsJSON(`{"context":"test","state":"FAILURE","isRequired":true}`, `{"hasNextPage":false}`))
			})
			rows := []model.PullRequest{{ID: "N1"}}
			err := client.enrich(context.Background(), rows)
			if calls != 2 {
				t.Fatalf("got %d calls, want 2", calls)
			}
			if broken {
				if err == nil || rows[0].CI != model.Unknown {
					t.Fatalf("incomplete checks must not look successful: %s, %v", rows[0].CI, err)
				}
			} else if err != nil || rows[0].CI != model.Fail {
				t.Fatalf("required failure on second page was lost: %s, %v", rows[0].CI, err)
			}
		})
	}
}

func TestRequiredChecksFromBaseBranch(t *testing.T) {
	for _, tc := range []struct {
		name    string
		baseRef string
	}{
		{"branch protection", `{"branchProtectionRule":{"requiredStatusCheckContexts":["test"]}}`},
		{"active ruleset", `{"rules":{"nodes":[{"parameters":{"requiredStatusChecks":[{"context":"test"}]}}],"pageInfo":{"hasNextPage":false}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Query string `json:"query"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatal(err)
				}
				for _, field := range []string{"baseRef", "requiredStatusCheckContexts", "RequiredStatusChecksParameters", "requiredStatusChecks"} {
					if !strings.Contains(req.Query, field) {
						t.Errorf("missing requirements query field %s", field)
					}
				}
				_, _ = fmt.Fprintf(w, `{"data":{"nodes":[{"id":"N1"}],"ci0":{"baseRef":%s,"commits":{"nodes":[{"commit":{"statusCheckRollup":null}}]}}}}`, tc.baseRef)
			})
			rows := []model.PullRequest{{ID: "N1", CI: model.Pass}}
			if err := client.enrich(context.Background(), rows); err != nil {
				t.Fatal(err)
			}
			if rows[0].CI != model.Missing {
				t.Fatalf("got %s, want MISSING", rows[0].CI)
			}
		})
	}
}

func TestRequiredChecksPagination(t *testing.T) {
	for _, mode := range []string{"complete", "error", "missing cursor", "repeated cursor"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls > 2 {
					t.Fatal("pagination did not stop")
				}
				var req struct {
					Query string `json:"query"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatal(err)
				}
				if calls == 1 {
					cursor := "next"
					if mode == "missing cursor" {
						cursor = ""
					}
					_, _ = fmt.Fprintf(w, `{"data":{"nodes":[{"id":"N1"}],"ci0":{"baseRef":{"rules":{"nodes":[],"pageInfo":{"hasNextPage":true,"endCursor":%q}}},"commits":{"nodes":[{"commit":{"statusCheckRollup":{"contexts":{"nodes":[{"context":"build","state":"SUCCESS"}]}}}}]}}}}`, cursor)
					return
				}
				if !strings.Contains(req.Query, `rules(first:100,after:"next")`) {
					t.Error("missing rules pagination cursor")
				}
				if mode == "error" {
					_, _ = w.Write([]byte(`{"data":{"ci0":null},"errors":[{"message":"unavailable"}]}`))
					return
				}
				_, _ = fmt.Fprintf(w, `{"data":{"ci0":{"baseRef":{"rules":{"nodes":[{"parameters":{"requiredStatusChecks":[{"context":"test"}]}}],"pageInfo":{"hasNextPage":%t,"endCursor":"next"}}}}}}`, mode == "repeated cursor")
			})
			rows := []model.PullRequest{{ID: "N1", CI: model.Pass}}
			err := client.enrich(context.Background(), rows)
			if mode == "complete" {
				if err != nil || rows[0].CI != model.Missing {
					t.Fatalf("lost required check on second page: %s, %v", rows[0].CI, err)
				}
			} else if err == nil || rows[0].CI != model.Unknown {
				t.Fatalf("incomplete rules must not look successful or missing: %s, %v", rows[0].CI, err)
			}
		})
	}
}
