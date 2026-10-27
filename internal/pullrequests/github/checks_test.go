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
