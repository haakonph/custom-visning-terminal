package model

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

func stackPR(n int) PullRequest {
	return PullRequest{Repository: "org/repo", Number: n, URL: fmt.Sprintf("https://github.com/org/repo/pull/%d", n), HeadRef: fmt.Sprintf("branch-%d", n)}
}

func TestPRStacks(t *testing.T) {
	a, b, c, unrelated := stackPR(1), stackPR(2), stackPR(3), stackPR(4)
	b.BaseRef = a.HeadRef
	c.Body = "Depends on #2."
	rows := []PullRequest{c, unrelated, b, a, a}
	got := PRStacks(rows)
	want := map[string]StackPosition{a.Key(): {1, 3}, b.Key(): {2, 3}, c.Key(): {3, 3}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if !reflect.DeepEqual(PRStacks([]PullRequest{a, b, unrelated, c}), want) {
		t.Fatal("input order changed numbering")
	}
}

func TestPRStackLinksAndCycles(t *testing.T) {
	a, b, c := stackPR(1), stackPR(2), stackPR(3)
	b.Repository = "other/repo"
	b.URL = "https://github.com/other/repo/pull/2"
	b.Body = a.URL
	a.Body = b.URL // Mutual references must terminate and remain stable.
	c.Body = "Not a reference: #20, path/#1, issue-#2."
	got := PRStacks([]PullRequest{a, b, c})
	if len(got) != 2 || got[a.Key()].Total != 2 || got[b.Key()].Position == got[a.Key()].Position {
		t.Fatalf("bad cycle: %v", got)
	}
	if !reflect.DeepEqual(got, PRStacks([]PullRequest{c, b, a})) {
		t.Fatal("cycle numbering unstable")
	}
}

func TestPRStacksUseNewestObservationAndHost(t *testing.T) {
	a, b := stackPR(1), stackPR(2)
	old := b
	old.BaseRef = a.HeadRef
	b.UpdatedAt = time.Now()
	if got := PRStacks([]PullRequest{b, a, old}); len(got) != 0 {
		t.Fatalf("stale dependency: %v", got)
	}
	b.BaseRef = a.HeadRef
	b.URL = "https://enterprise.example/org/repo/pull/2"
	b.Body = "#1"
	if got := PRStacks([]PullRequest{a, b}); len(got) != 0 {
		t.Fatalf("cross-host dependency: %v", got)
	}
}
