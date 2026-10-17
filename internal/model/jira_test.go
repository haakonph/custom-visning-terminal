package model

import (
	"reflect"
	"testing"
)

func TestJiraKeys(t *testing.T) {
	p := PullRequest{Title: "ABC-12 fix ABC-12", Body: "https://jira.example/browse/XYZ-34 and xABC-56 ABC-7suffix", HeadRef: "feature/TEAM_2-90"}
	want := []string{"ABC-12", "XYZ-34", "TEAM_2-90"}
	if got := p.JiraKeys(); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if p.HasJiraKey("ABC-1") {
		t.Fatal("matched a partial issue number")
	}
	if !p.HasJiraKey("ABC-12") {
		t.Fatal("missing exact issue key")
	}
}
