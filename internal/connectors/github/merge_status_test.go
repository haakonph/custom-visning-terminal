package github

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestMergeStatusFromGraphQL(t *testing.T) {
	for _, tc := range []struct {
		name, fields, auto, queue string
		position                  int
	}{
		{"enabled and queued", `,"autoMergeRequest":{"enabledAt":"2026-10-19T10:00:00Z"},"mergeQueueEntry":{"position":3,"state":"AWAITING_CHECKS"}`, "ON", "AWAITING_CHECKS", 3},
		{"disabled", `,"autoMergeRequest":null,"mergeQueueEntry":null`, "OFF", "NONE", 0},
		{"missing", "", "UNKNOWN", "UNKNOWN", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Query string `json:"query"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				for _, field := range []string{"autoMergeRequest", "mergeQueueEntry", "position", "state"} {
					if !strings.Contains(req.Query, field) {
						t.Errorf("missing query field %s", field)
					}
				}
				_, _ = w.Write([]byte(`{"data":{"nodes":[{"id":"N1"` + tc.fields + `}]}}`))
			})
			rows := []model.PullRequest{{ID: "N1"}}
			if err := client.enrich(context.Background(), rows); err != nil {
				t.Fatal(err)
			}
			p := rows[0]
			if string(p.AutoMerge) != tc.auto || string(p.MergeQueue) != tc.queue || p.QueuePosition != tc.position {
				t.Fatalf("%+v", p)
			}
		})
	}
}
