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
	c.BaseRef = b.HeadRef
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

func TestPRStackBranchCycles(t *testing.T) {
	a, b, c := stackPR(1), stackPR(2), stackPR(3)
	b.BaseRef = a.HeadRef
	a.BaseRef = b.HeadRef // Cyclic branches must terminate and remain stable.
	got := PRStacks([]PullRequest{a, b, c})
	if len(got) != 2 || got[a.Key()].Total != 2 || got[b.Key()].Position == got[a.Key()].Position {
		t.Fatalf("bad cycle: %v", got)
	}
	if !reflect.DeepEqual(got, PRStacks([]PullRequest{c, b, a})) {
		t.Fatal("cycle numbering unstable")
	}
}

func TestPRStacksIgnoreBodyReferences(t *testing.T) {
	for _, body := range []string{"Depends on #1.", "Depends on https://github.com/org/repo/pull/1."} {
		t.Run(body, func(t *testing.T) {
			a, b, c := stackPR(1), stackPR(2), stackPR(3)
			b.BaseRef = a.HeadRef
			c.BaseRef = "main"
			c.Body = body
			want := map[string]StackPosition{a.Key(): {1, 2}, b.Key(): {2, 2}}
			if got := PRStacks([]PullRequest{a, b, c}); !reflect.DeepEqual(got, want) {
				t.Fatalf("body reference changed stack: got %v, want %v", got, want)
			}
		})
	}
}

func TestPRStacksStayWithinRepository(t *testing.T) {
	a, b, c, d := stackPR(1), stackPR(2), stackPR(3), stackPR(4)
	b.BaseRef = a.HeadRef
	c.Repository, d.Repository = "other/repo", "other/repo"
	c.URL = "https://github.com/other/repo/pull/3"
	d.URL = "https://github.com/other/repo/pull/4"
	c.HeadRef = a.HeadRef // The two repos happen to use the same branch names.
	d.BaseRef = c.HeadRef
	c.Body = b.URL
	b.Body = d.URL
	want := map[string]StackPosition{a.Key(): {1, 2}, b.Key(): {2, 2}, c.Key(): {1, 2}, d.Key(): {2, 2}}
	if got := PRStacks([]PullRequest{a, b, c, d}); !reflect.DeepEqual(got, want) {
		t.Fatalf("repositories joined into one stack: got %v, want %v", got, want)
	}
}

func TestPRStacksIgnoreParallelAndMissingBranches(t *testing.T) {
	a, b, c := stackPR(1), stackPR(2), stackPR(3)
	a.BaseRef, b.BaseRef = "main", "main"
	c.HeadRef = ""
	if got := PRStacks([]PullRequest{a, b, c}); len(got) != 0 {
		t.Fatalf("unrelated branches formed a stack: %v", got)
	}
	a.BaseRef = a.HeadRef
	if got := PRStacks([]PullRequest{a, b, c}); len(got) != 0 {
		t.Fatalf("self-reference formed a stack: %v", got)
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

func TestPRStackIndexPreservesCaseAndDuplicateBranchRules(t *testing.T) {
	a, b, c := stackPR(1), stackPR(2), stackPR(3)
	b.Repository = "ORG/REPO"
	b.URL = "https://GITHUB.COM/ORG/REPO/pull/2"
	b.BaseRef = a.HeadRef
	c.HeadRef = a.HeadRef
	got := PRStacks([]PullRequest{a, b, c})
	if len(got) != 3 || got[b.Key()].Total != 3 {
		t.Fatal("case-insensitive repository or duplicate branch lost", got)
	}
	b.BaseRef = "BRANCH-1"
	if got := PRStacks([]PullRequest{a, b, c}); len(got) != 0 {
		t.Fatal("branch matching must be case sensitive", got)
	}
	if stackIdentity("Σ/K") != stackIdentity("ς/k") {
		t.Fatal("repository index changed EqualFold semantics")
	}
}
