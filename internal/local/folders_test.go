package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestIssueFolderSeparateCheckoutReuseAndPreferences(t *testing.T) {
	w, p, source := fixture(t)
	run(t, source, "branch", "-M", "main")
	ctx := context.Background()
	f, err := w.CreateIssue(ctx, p.Repository, "TEST-42", "Oppgave med mellomrom")
	if err != nil {
		t.Fatal(err)
	}
	if run(t, f.Path, "symbolic-ref", "--short", "HEAD") != "custom-visning/jira/test-42" {
		t.Fatal("missing issue branch")
	}
	dirty := filepath.Join(f.Path, "local-file")
	if err := os.WriteFile(dirty, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	f.Favorite, f.Hidden = true, true
	if err := w.SaveFolder(f); err != nil {
		t.Fatal(err)
	}
	again, err := w.CreateIssue(ctx, p.Repository, "TEST-42", "Changed title")
	if err != nil || again.Path != f.Path || !again.Favorite || !again.Hidden {
		t.Fatalf("reuse lost metadata: %#v %v", again, err)
	}
	if data, err := os.ReadFile(dirty); err != nil || string(data) != "keep" {
		t.Fatal("modified checkout overwritten")
	}
	restarted, err := New(w.Repositories.Dir, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := restarted.Folders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range rows {
		if row.Path == f.Path {
			found = true
			if !row.Favorite || !row.Hidden || row.IssueKey != "TEST-42" {
				t.Fatalf("metadata lost: %#v", row)
			}
		}
	}
	if !found {
		t.Fatal("issue folder missing after restart")
	}
	if run(t, source, "branch", "--show-current") != "main" {
		t.Fatal("registered checkout moved")
	}
}

func TestIssueFolderRefusesUnmanagedPathAndInvalidInput(t *testing.T) {
	w, p, source := fixture(t)
	run(t, source, "branch", "-M", "main")
	path := filepath.Join(w.Repositories.Dir, "jira", "acme", "repo", "test-42")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := w.CreateIssue(context.Background(), p.Repository, "TEST-42", "title"); err == nil {
		t.Fatal("reused unmanaged path")
	}
	for _, key := range []string{"../escape", "TEST-42/../../x", "-TEST-42", "TEST-42\n"} {
		if _, err := w.CreateIssue(context.Background(), p.Repository, key, "title"); err == nil {
			t.Fatalf("accepted %q", key)
		}
	}
}

func TestFoldersFindRegisteredWorktreesWithSpaces(t *testing.T) {
	w, p, source := fixture(t)
	w.Repositories.Paths = []string{source}
	run(t, source, "remote", "add", "origin", "https://github.com/"+p.Repository+".git")
	path := filepath.Join(t.TempDir(), "work tree with spaces")
	run(t, source, "worktree", "add", "-b", "extra", path)
	rows, err := w.Folders(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, f := range rows {
		paths[f.Path] = true
	}
	if !paths[source] || !paths[path] {
		t.Fatalf("missing registered checkouts: %#v", rows)
	}
}
