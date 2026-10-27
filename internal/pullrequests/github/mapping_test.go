package github

import (
	"encoding/json"
	"testing"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestDetailsPreserveStackBranches(t *testing.T) {
	var d details
	if err := json.Unmarshal([]byte(`{"baseRefName":"feature/base","headRefName":"feature/child"}`), &d); err != nil {
		t.Fatal(err)
	}
	var p model.PullRequest
	applyDetails(&p, &d)
	if p.BaseRef != "feature/base" || p.HeadRef != "feature/child" {
		t.Fatalf("lost branches: %+v", p)
	}
}
