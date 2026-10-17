package model

import (
	"testing"
	"time"
)

func TestMergeDeduplicatesAndSorts(t *testing.T) {
	old := []PullRequest{{URL: "a", Title: "old"}, {URL: "b"}}
	fresh := []PullRequest{{URL: "a", Title: "new", UpdatedAt: time.Now()}}
	got := Merge(old, fresh)
	if len(got) != 2 || got[0].URL != "a" || got[0].Title != "new" {
		t.Fatalf("%+v", got)
	}
	if old[0].Title != "old" {
		t.Fatal("mutated cached slice")
	}
}
