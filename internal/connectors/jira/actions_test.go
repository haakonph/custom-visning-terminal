package jira

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestWorkflowActions(t *testing.T) {
	paths := []string{}
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatal("missing auth")
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /jira/rest/api/2/issue/APP-1/transitions":
			w.Write([]byte(`{"transitions":[{"id":"42","name":"Review","to":{"name":"QA"}}]}`))
		case "POST /jira/rest/api/2/issue/APP-1/transitions":
			var body struct{ Transition struct{ ID string } }
			json.NewDecoder(r.Body).Decode(&body)
			if body.Transition.ID != "42" {
				t.Fatal("wrong workflow ID")
			}
			w.WriteHeader(204)
		case "GET /jira/rest/api/2/myself":
			w.Write([]byte(`{"name":"me"}`))
		case "PUT /jira/rest/api/2/issue/APP-1/assignee":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["name"] != "me" && body["name"] != nil {
				t.Fatal("wrong assignee")
			}
			w.WriteHeader(204)
		case "GET /jira/rest/api/2/issue/APP-1":
			w.Write([]byte(`{"fields":{"assignee":{"name":"me"}}}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()
	transitions, err := c.Transitions(ctx, "APP-1")
	if err != nil || len(transitions) != 1 || transitions[0].To.Name != "QA" {
		t.Fatalf("%v %v", transitions, err)
	}
	if err := c.Transition(ctx, "APP-1", transitions[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := c.Assign(ctx, "APP-1", true); err != nil {
		t.Fatal(err)
	}
	if err := c.Assign(ctx, "APP-1", false); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 7 {
		t.Fatalf("requests: %v", paths)
	}
}
func TestActionsRejectInvalidKeysAndHideResponse(t *testing.T) {
	calls := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(403)
		w.Write([]byte("secret internal details"))
	})
	for _, key := range []string{"../APP-1", "APP-1/assignee", ""} {
		if _, err := c.Transitions(context.Background(), key); err == nil {
			t.Fatal("invalid key accepted")
		}
	}
	if calls != 0 {
		t.Fatal("invalid key made request")
	}
	err := c.Transition(context.Background(), "APP-1", "42")
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal("unsafe error")
	}
}
func TestRemoveMePreservesSomeoneElse(t *testing.T) {
	writes := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			writes++
			w.WriteHeader(204)
			return
		}
		if strings.HasSuffix(r.URL.Path, "myself") {
			w.Write([]byte(`{"name":"me"}`))
		} else {
			w.Write([]byte(`{"fields":{"assignee":{"name":"other"}}}`))
		}
	})
	if err := c.Assign(context.Background(), "APP-1", false); err == nil || writes != 0 {
		t.Fatal("removed another assignee")
	}
}
