package local

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

var commitPattern = regexp.MustCompile(`^[a-fA-F0-9]{40}([a-fA-F0-9]{24})?$`)

type Status struct {
	Path     string `json:"path"`
	Head     string `json:"head"`
	Modified bool   `json:"-"`
	Outdated bool   `json:"-"`
	Error    string `json:"-"`
	Note     string `json:"-"`
}

func (s Status) String() string {
	if s.Error != "" {
		return "Error: " + s.Error
	}
	if s.Path == "" {
		return "Not downloaded"
	}
	v := "Ready"
	if s.Modified {
		v = "Modified"
	}
	if s.Outdated {
		v += " · Outdated"
	}
	if s.Note != "" {
		v += " · " + s.Note
	}
	return v
}

type WorktreeManager struct {
	Repositories *RepositoryManager
	mu           sync.Mutex
	entries      map[string]Status
}

func New(dir string, c Config) (*WorktreeManager, error) {
	var err error
	dir, err = filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	r := &RepositoryManager{Dir: dir, Paths: c.RepositoryPaths, mappings: map[string]string{}}
	w := &WorktreeManager{Repositories: r, entries: map[string]Status{}}
	for _, item := range []struct {
		name  string
		value any
	}{{"repositories.json", &r.mappings}, {"worktrees.json", &w.entries}} {
		b, err := os.ReadFile(filepath.Join(dir, item.name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, item.value); err != nil {
			return nil, fmt.Errorf("invalid %s: %w", item.name, err)
		}
	}
	return w, nil
}
func key(p model.PullRequest) string {
	return fmt.Sprintf("%s#%d", strings.ToLower(p.Repository), p.Number)
}
func (w *WorktreeManager) path(p model.PullRequest) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if s := w.entries[key(p)]; s.Path != "" && filepath.IsAbs(s.Path) {
		return s.Path
	}
	return filepath.Join(w.Repositories.Dir, "worktrees", filepath.FromSlash(strings.ToLower(p.Repository)), fmt.Sprintf("pr-%d", p.Number))
}
func common(ctx context.Context, path string) (string, error) {
	v, err := git(ctx, path, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(v) {
		v = filepath.Join(path, v)
	}
	return filepath.EvalSymlinks(v)
}
func inspect(ctx context.Context, path, repo, head string) (Status, error) {
	s := Status{Path: path}
	// A directory nested inside another worktree is not itself a worktree.
	top, err := git(ctx, path, "rev-parse", "--show-toplevel")
	if err != nil {
		return s, err
	}
	a, e := filepath.EvalSymlinks(path)
	b, e2 := filepath.EvalSymlinks(top)
	if e != nil || e2 != nil || a != b {
		return s, fmt.Errorf("%s is not a worktree root", path)
	}
	a, err = common(ctx, path)
	if err != nil {
		return s, err
	}
	b, err = common(ctx, repo)
	if err != nil || a != b {
		return s, fmt.Errorf("%s belongs to another repository; left untouched", path)
	}
	s.Head, err = git(ctx, path, "rev-parse", "HEAD")
	if err != nil {
		return s, err
	}
	dirty, err := git(ctx, path, "status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return s, err
	}
	s.Modified = dirty != ""
	s.Outdated = head != "" && s.Head != head
	return s, nil
}

// Status performs local reads only; callers cache it outside rendering.
func (w *WorktreeManager) Status(ctx context.Context, p model.PullRequest) Status {
	if !validIdentity(p.Repository) || p.Number <= 0 {
		return Status{Error: "invalid PR identity"}
	}
	path := w.path(p)
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return Status{}
	}
	// Validate GitHub identity independently of persisted repository mappings.
	if !w.Repositories.matches(ctx, path, strings.ToLower(p.Repository)) {
		return Status{Error: "worktree remote no longer matches repository"}
	}
	s, err := inspect(ctx, path, path, p.HeadOID)
	if err != nil {
		s.Error = err.Error()
	}
	return s
}
func (w *WorktreeManager) Prepare(ctx context.Context, p model.PullRequest) (Status, error) {
	if !validIdentity(p.Repository) || p.Number <= 0 {
		return Status{}, fmt.Errorf("invalid PR identity")
	}
	id := strings.ToLower(p.Repository)
	unlock := w.Repositories.lock(id)
	defer unlock()
	if _, err := exec.LookPath("git"); err != nil {
		return Status{}, fmt.Errorf("install Git and add it to PATH: %w", err)
	}
	path := w.path(p)
	if _, err := os.Lstat(path); err == nil {
		if !w.Repositories.matches(ctx, path, id) {
			return Status{}, fmt.Errorf("%s is not a matching GitHub worktree; left untouched", path)
		}
		physical, e := common(ctx, path)
		if e != nil {
			return Status{}, e
		}
		unlockPhysical := w.Repositories.lock("path:" + physical)
		defer unlockPhysical()
		s, e := inspect(ctx, path, path, p.HeadOID)
		if e != nil {
			return s, e
		}
		return w.reuse(ctx, p, s)
	} else if !os.IsNotExist(err) {
		return Status{}, err
	}
	repo, err := w.Repositories.prepare(ctx, id)
	if err != nil {
		return Status{}, err
	}
	physical, err := common(ctx, repo)
	if err != nil {
		return Status{}, err
	}
	// Multiple GitHub remotes can identify the same physical repository.
	unlockPhysical := w.Repositories.lock("path:" + physical)
	defer unlockPhysical()
	if _, err = os.Lstat(path); err == nil {
		s, e := inspect(ctx, path, repo, p.HeadOID)
		if e != nil {
			return s, e
		}
		return w.reuse(ctx, p, s)
	} else if !os.IsNotExist(err) {
		return Status{}, err
	}
	have, err := prCommit(ctx, repo, p)
	if err != nil {
		return Status{}, err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return Status{}, err
	}
	if _, err = git(ctx, repo, "worktree", "add", "--detach", "--", path, have); err != nil {
		return Status{}, fmt.Errorf("cannot add %s; inspect git worktree list and repair stale metadata manually: %w", path, err)
	}
	s, err := inspect(ctx, path, repo, p.HeadOID)
	if err != nil {
		return s, err
	}
	return s, w.remember(p, s)
}

// prCommit resolves the dashboard head locally, fetching the base PR ref
// (including forks) only when necessary. It never changes a working tree.
func prCommit(ctx context.Context, repo string, p model.PullRequest) (string, error) {
	if commitPattern.MatchString(p.HeadOID) {
		if have, err := git(ctx, repo, "rev-parse", "--verify", p.HeadOID+"^{commit}"); err == nil {
			return have, nil
		}
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return "", fmt.Errorf("install GitHub CLI and run gh auth login before fetching PR commits: %w", err)
	}
	ref := fmt.Sprintf("refs/custom-visning/%s/pr-%d", strings.ToLower(p.Repository), p.Number)
	if _, err := git(ctx, repo, "-c", "credential.helper=", "-c", "credential.helper=!gh auth git-credential", "fetch", "--no-tags", "--", "https://github.com/"+strings.ToLower(p.Repository)+".git", fmt.Sprintf("+refs/pull/%d/head:%s", p.Number, ref)); err != nil {
		return "", fmt.Errorf("PR #%d fetch failed (deleted PR, access, network or gh authentication): %w", p.Number, err)
	}
	return git(ctx, repo, "rev-parse", ref+"^{commit}")
}

// Only fast-forward a clean detached HEAD. No reset, force checkout, rebase,
// branch movement, or automatic stash is used. Unsafe/unavailable updates still
// open the existing checkout and explain why it remains outdated.
func (w *WorktreeManager) reuse(ctx context.Context, p model.PullRequest, s Status) (Status, error) {
	if s.Modified || !s.Outdated || !commitPattern.MatchString(p.HeadOID) {
		return s, w.remember(p, s)
	}
	branch, err := git(ctx, s.Path, "rev-parse", "--symbolic-full-name", "HEAD")
	if err != nil {
		return s, err
	}
	if branch != "HEAD" {
		s.Note = "on a branch; update manually"
		return s, w.remember(p, s)
	}
	for _, name := range []string{"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "BISECT_LOG", "BISECT_START", "rebase-merge", "rebase-apply", "sequencer"} {
		marker, err := git(ctx, s.Path, "rev-parse", "--git-path", name)
		if err != nil {
			return s, err
		}
		if !filepath.IsAbs(marker) {
			marker = filepath.Join(s.Path, marker)
		}
		if _, err = os.Lstat(marker); !os.IsNotExist(err) {
			s.Note = "Git operation in progress; update manually"
			return s, w.remember(p, s)
		}
	}
	target, err := prCommit(ctx, s.Path, p)
	if err != nil {
		if ctx.Err() != nil {
			return s, ctx.Err()
		}
		s.Note = "update unavailable: " + err.Error()
		return s, w.remember(p, s)
	}
	if target != p.HeadOID {
		s.Note = "PR changed again; refresh before updating"
		return s, w.remember(p, s)
	}
	if _, err = git(ctx, s.Path, "merge-base", "--is-ancestor", s.Head, target); err != nil {
		s.Note = "local commits or rewritten history; update manually"
		return s, w.remember(p, s)
	}
	// Recheck after fetching, since another tool may have modified the checkout.
	current, err := inspect(ctx, s.Path, s.Path, p.HeadOID)
	if err != nil {
		return s, err
	}
	branch, err = git(ctx, s.Path, "rev-parse", "--symbolic-full-name", "HEAD")
	if err != nil {
		return current, err
	}
	if current.Modified || current.Head != s.Head || branch != "HEAD" {
		return current, w.remember(p, current)
	}
	if _, err = git(ctx, s.Path, "checkout", "--detach", "--no-overwrite-ignore", target); err != nil {
		if ctx.Err() != nil {
			return s, ctx.Err()
		}
		s.Note = "automatic update failed; checkout preserved; inspect Git status"
		return s, w.remember(p, s)
	}
	updated, err := inspect(ctx, s.Path, s.Path, p.HeadOID)
	if err != nil {
		return updated, err
	}
	return updated, w.remember(p, updated)
}

func (w *WorktreeManager) remember(p model.PullRequest, s Status) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.entries == nil {
		w.entries = map[string]Status{}
	}
	w.entries[key(p)] = s
	return writeJSON(filepath.Join(w.Repositories.Dir, "worktrees.json"), w.entries)
}
