package model

import (
	"reflect"
	"regexp"
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

func TestCustomJiraPattern(t *testing.T) {
	pattern := regexp.MustCompile(`KAKE-[0-9]{3,5}`)
	p := PullRequest{
		Title:   "KAKE-12 KAKE-123 KAKE-123456 ABC-123 xKAKE-456 KAKE-789suffix",
		Body:    "https://jira.example/browse/KAKE-1234 KAKE-123",
		HeadRef: "feature/KAKE-12345",
	}
	want := []string{"KAKE-123", "KAKE-1234", "KAKE-12345"}
	if got := p.JiraKeys(pattern); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if p.HasJiraKey("KAKE-12", pattern) || p.HasJiraKey("ABC-123", pattern) {
		t.Fatal("matched a key outside the custom pattern")
	}
	if !p.HasJiraKey("kake-12345", pattern) {
		t.Fatal("missing custom key")
	}
}
