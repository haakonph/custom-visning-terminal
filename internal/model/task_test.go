package model

import "testing"

func TestTaskReferenceIdentityAndReviewContext(t *testing.T) {
	task := Task{Title: "Release"}
	task.Add(TaskReference{Provider: "github", URL: "https://github.com/org/one/pull/1", Role: "review", Title: "Old"})
	task.Add(TaskReference{Provider: "github", URL: "https://github.com/org/two/pull/1", Title: "Other repository"})
	task.Add(TaskReference{Provider: "jira", URL: "https://jira.example/browse/ABC-1"})
	task.Add(TaskReference{Provider: "github", URL: "https://github.com/org/one/pull/1", Title: "Updated"})
	if len(task.References) != 3 || task.References[0].Title != "Updated" || task.References[0].Role != "review" {
		t.Fatal("references must deduplicate by full provider URL and preserve review context")
	}
}
