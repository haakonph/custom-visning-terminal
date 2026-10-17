package local

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestIdentity(t *testing.T) {
	for _, v := range []string{"git@github.com:Owner/Repo.git", "https://github.com/Owner/Repo.git", "ssh://git@github.com/Owner/Repo.git"} {
		got, err := Identity(v)
		if err != nil || got != "owner/repo" {
			t.Fatalf("%s: %s %v", v, got, err)
		}
	}
	for _, v := range []string{"https://evil.com/o/r", "/tmp/repo", "https://github.com/../x", "https://github.com/o/r/extra", "https://github.com/o/r?token=secret"} {
		if _, err := Identity(v); err == nil {
			t.Fatal(v)
		}
	}
}
func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	b, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s: %v", args, b, err)
	}
	return strings.TrimSpace(string(b))
}
func fixture(t *testing.T) (*WorktreeManager, model.PullRequest, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git required")
	}
	root := t.TempDir()
	source := filepath.Join(root, "remote.git")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(root, "gitconfig"))
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	run(t, source, "init")
	run(t, source, "commit", "--allow-empty", "-m", "first")
	sha := run(t, source, "rev-parse", "HEAD")
	run(t, source, "update-ref", "refs/pull/1/head", sha)
	run(t, source, "update-ref", "refs/pull/2/head", sha)
	run(t, source, "config", "--global", "url."+source+".insteadOf", "https://github.com/acme/repo.git")
	// The fake gh is deliberately offline; authentication is unnecessary for the
	// local URL rewrite, but clone must still validate the executable exists.
	bin := filepath.Join(root, "bin")
	_ = os.Mkdir(bin, 0700)
	_ = os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\nexit 0\n"), 0700)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	w, err := New(filepath.Join(root, "data"), Defaults())
	if err != nil {
		t.Fatal(err)
	}
	return w, model.PullRequest{Repository: "acme/repo", Number: 1, URL: "https://github.com/acme/repo/pull/1", HeadOID: sha}, source
}
func TestCloneReuseAndConcurrentWorktrees(t *testing.T) {
	w, p, _ := fixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			q := p
			if i%2 == 1 {
				q.Number = 2
			}
			_, err := w.Prepare(ctx, q)
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	repo, err := w.Repositories.Prepare(ctx, p.Repository)
	if err != nil {
		t.Fatal(err)
	}
	if run(t, repo, "rev-parse", "--is-bare-repository") != "true" {
		t.Fatal("cache not bare")
	}
	first, err := w.Prepare(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	second, err := w.Prepare(ctx, p)
	if err != nil || first.Path != second.Path {
		t.Fatalf("not reused: %v", err)
	}
	if commonA, _ := common(ctx, first.Path); commonA != repo {
		t.Fatalf("objects not shared: %s %s", commonA, repo)
	}
	loaded, err := New(w.Repositories.Dir, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	again, err := loaded.Prepare(ctx, p)
	if err != nil || again.Path != first.Path {
		t.Fatal("persisted worktree not reused", err)
	}
}
func TestDirtyAndOutdatedUntouched(t *testing.T) {
	w, p, source := fixture(t)
	ctx := context.Background()
	s, err := w.Prepare(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(s.Path, "local.txt")
	_ = os.WriteFile(file, []byte("keep me"), 0600)
	run(t, source, "commit", "--allow-empty", "-m", "new head")
	p.HeadOID = run(t, source, "rev-parse", "HEAD")
	run(t, source, "update-ref", "refs/pull/1/head", p.HeadOID)
	updated, err := w.Prepare(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Modified || !updated.Outdated {
		t.Fatalf("missing independent status: %+v", updated)
	}
	if updated.Head == p.HeadOID {
		t.Fatal("existing worktree moved")
	}
	b, _ := os.ReadFile(file)
	if string(b) != "keep me" {
		t.Fatal("local changes lost")
	}
	if status := w.Status(ctx, p); !status.Modified || !status.Outdated {
		t.Fatalf("status: %+v", status)
	}
}
func TestRegisteredRepositoryAndMissingPaths(t *testing.T) {
	w, p, source := fixture(t)
	run(t, source, "remote", "add", "origin", "git@github.com:acme/repo.git")
	w.Repositories.Paths = []string{filepath.Join(t.TempDir(), "deleted"), source}
	path, err := w.Repositories.Prepare(context.Background(), p.Repository)
	if err != nil || path != source {
		t.Fatal(path, err)
	}
	s, err := w.Prepare(context.Background(), p)
	if err != nil || s.Path == "" {
		t.Fatal(s, err)
	}
	if _, err = os.Stat(filepath.Join(w.Repositories.Dir, "repos", "acme", "repo.git")); !os.IsNotExist(err) {
		t.Fatal("unnecessary clone")
	}
}
func TestConflictsAndPartialFailure(t *testing.T) {
	w, p, _ := fixture(t)
	path := w.path(p)
	_ = os.MkdirAll(path, 0700)
	file := filepath.Join(path, "keep")
	_ = os.WriteFile(file, []byte("data"), 0600)
	if _, err := w.Prepare(context.Background(), p); err == nil {
		t.Fatal("accepted non-worktree directory")
	}
	q := p
	q.Number = 2
	if _, err := w.Prepare(context.Background(), q); err != nil {
		t.Fatal("other PR failed", err)
	}
	if b, _ := os.ReadFile(file); string(b) != "data" {
		t.Fatal("overwrote directory")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	q.Number = 3
	if _, err := w.Prepare(canceled, q); err == nil {
		t.Fatal("cancellation ignored")
	}
}
func TestFetchMissingPRCommit(t *testing.T) {
	w, p, source := fixture(t)
	if _, err := w.Repositories.Prepare(context.Background(), p.Repository); err != nil {
		t.Fatal(err)
	}
	run(t, source, "commit", "--allow-empty", "-m", "fork commit")
	p.HeadOID = run(t, source, "rev-parse", "HEAD")
	run(t, source, "update-ref", "refs/pull/1/head", p.HeadOID)
	s, err := w.Prepare(context.Background(), p)
	if err != nil || s.Head != p.HeadOID {
		t.Fatal(s, err)
	}
	p.Number = 999
	p.HeadOID = ""
	if _, err := w.Prepare(context.Background(), p); err == nil {
		t.Fatal("missing PR accepted")
	}
}
func TestEditorArguments(t *testing.T) {
	binary, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git required")
	}
	c := Defaults()
	c.Editors.Custom = Editor{binary, []string{"--project={path}", "literal; $(touch bad)", "{path}"}}
	l := EditorLauncher{c.Editors}
	path := filepath.Join(t.TempDir(), "space dir")
	cmd, err := l.Command("custom", path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{binary, "--project=" + path, "literal; $(touch bad)", path}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("%q != %q", cmd.Args, want)
	}
	c.Editors.VSCode.Command = binary
	l.Editors = c.Editors
	cmd, err = l.Command("vscode", path)
	if err != nil || !reflect.DeepEqual(cmd.Args, []string{binary, "--new-window", path}) {
		t.Fatal(cmd, err)
	}
	l.Editors.IntelliJ.Command = "custom-visning-no-such-editor"
	if _, err = l.Command("intellij", path); err == nil {
		t.Fatal("missing editor accepted")
	}
}
func TestConfigPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	c, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	c.RepositoryPaths = []string{"/tmp/repo"}
	c.Editors.Default = "custom"
	c.Editors.Custom = Editor{"tool", []string{"{path}"}}
	if err = writeJSON(path, c); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig(path)
	if err != nil || !reflect.DeepEqual(c, got) {
		t.Fatal(got, err)
	}
}

func TestStaleWorktreeMetadataPreserved(t *testing.T) {
	w, p, _ := fixture(t)
	s, err := w.Prepare(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	// Only remove our test-owned checkout, leaving stale Git metadata behind.
	if err = os.RemoveAll(s.Path); err != nil {
		t.Fatal(err)
	}
	if _, err = w.Prepare(context.Background(), p); err == nil {
		t.Fatal("stale metadata should require manual repair")
	}
	repo, err := w.Repositories.Prepare(context.Background(), p.Repository)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(run(t, repo, "worktree", "list", "--porcelain"), s.Path) {
		t.Fatal("metadata silently pruned")
	}
}
func TestMissingTools(t *testing.T) {
	w, p, _ := fixture(t)
	ctx := context.Background()
	bin := t.TempDir()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git required")
	}
	if err = os.Symlink(gitPath, filepath.Join(bin, "git")); err != nil {
		t.Skip("symlink unavailable")
	}
	t.Setenv("PATH", bin)
	if _, err = w.Repositories.Prepare(ctx, p.Repository); err == nil || !strings.Contains(err.Error(), "GitHub CLI") {
		t.Fatal("missing gh:", err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err = w.Repositories.Prepare(ctx, p.Repository); err == nil || !strings.Contains(err.Error(), "install Git") {
		t.Fatal("missing Git:", err)
	}
}

func TestExistingWorktreeSurvivesRepositoryPreferenceChange(t *testing.T) {
	w, p, source := fixture(t)
	s, err := w.Prepare(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	run(t, source, "remote", "add", "origin", "https://github.com/acme/repo.git")
	w.Repositories.Paths = []string{source}
	again, err := w.Prepare(context.Background(), p)
	if err != nil || again.Path != s.Path {
		t.Fatal("existing worktree not reused", again, err)
	}
}

func advancePR(t *testing.T, source string, p model.PullRequest) model.PullRequest {
	t.Helper()
	run(t, source, "commit", "--allow-empty", "-m", "new remote head")
	p.HeadOID = run(t, source, "rev-parse", "HEAD")
	run(t, source, "update-ref", "refs/pull/1/head", p.HeadOID)
	return p
}
func TestCleanWorktreeAutomaticallyFastForwards(t *testing.T) {
	w, p, source := fixture(t)
	ctx := context.Background()
	first, err := w.Prepare(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	p = advancePR(t, source, p)
	updated, err := w.Prepare(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Path != first.Path || updated.Head != p.HeadOID || updated.Outdated || updated.Modified {
		t.Fatalf("not updated: %+v", updated)
	}
	if _, err = git(ctx, updated.Path, "symbolic-ref", "-q", "HEAD"); err == nil {
		t.Fatal("HEAD should remain detached")
	}
}
func TestLocalCommitsAreNotReplaced(t *testing.T) {
	w, p, source := fixture(t)
	ctx := context.Background()
	first, err := w.Prepare(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	run(t, first.Path, "commit", "--allow-empty", "-m", "my local work")
	localHead := run(t, first.Path, "rev-parse", "HEAD")
	p = advancePR(t, source, p)
	got, err := w.Prepare(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Head != localHead || !got.Outdated || !strings.Contains(got.Note, "local commits") {
		t.Fatalf("local commits not preserved: %+v", got)
	}
}
func TestAttachedBranchIsNotMovedOrDetached(t *testing.T) {
	w, p, source := fixture(t)
	ctx := context.Background()
	first, err := w.Prepare(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	run(t, first.Path, "checkout", "-b", "my-branch")
	p = advancePR(t, source, p)
	got, err := w.Prepare(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Head != first.Head || !got.Outdated || run(t, first.Path, "symbolic-ref", "--short", "HEAD") != "my-branch" {
		t.Fatalf("branch changed: %+v", got)
	}
}
func TestFailedUpdateStillReusesCheckout(t *testing.T) {
	w, p, source := fixture(t)
	ctx := context.Background()
	first, err := w.Prepare(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	p = advancePR(t, source, p)
	run(t, source, "update-ref", "-d", "refs/pull/1/head")
	got, err := w.Prepare(ctx, p)
	if err != nil {
		t.Fatal("existing checkout should still open", err)
	}
	if got.Head != first.Head || !got.Outdated || !strings.Contains(got.Note, "update unavailable") {
		t.Fatalf("missing failure notice: %+v", got)
	}
}
func TestUpdateDoesNotOverwriteIgnoredFiles(t *testing.T) {
	w, p, source := fixture(t)
	ctx := context.Background()
	_ = os.WriteFile(filepath.Join(source, ".gitignore"), []byte("private.txt\n"), 0600)
	run(t, source, "add", ".gitignore")
	run(t, source, "commit", "-m", "ignore local file")
	p.HeadOID = run(t, source, "rev-parse", "HEAD")
	run(t, source, "update-ref", "refs/pull/1/head", p.HeadOID)
	first, err := w.Prepare(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(first.Path, "private.txt")
	_ = os.WriteFile(file, []byte("my ignored data"), 0600)
	_ = os.WriteFile(filepath.Join(source, "private.txt"), []byte("remote content"), 0600)
	run(t, source, "add", "-f", "private.txt")
	run(t, source, "commit", "-m", "track file")
	p.HeadOID = run(t, source, "rev-parse", "HEAD")
	run(t, source, "update-ref", "refs/pull/1/head", p.HeadOID)
	got, err := w.Prepare(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(file)
	if err != nil || string(b) != "my ignored data" || got.Head != first.Head || !got.Outdated {
		t.Fatalf("ignored file changed: %+v, %q, %v", got, b, err)
	}
}
func TestGitOperationPreventsUpdate(t *testing.T) {
	w, p, source := fixture(t)
	ctx := context.Background()
	first, err := w.Prepare(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	p = advancePR(t, source, p)
	marker := run(t, first.Path, "rev-parse", "--git-path", "BISECT_LOG")
	if !filepath.IsAbs(marker) {
		marker = filepath.Join(first.Path, marker)
	}
	if err = os.WriteFile(marker, []byte("in progress"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := w.Prepare(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Head != first.Head || !strings.Contains(got.Note, "in progress") {
		t.Fatalf("operation not preserved: %+v", got)
	}
}
