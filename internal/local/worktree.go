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
		return s, w.remember(p, s)
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
		return s, w.remember(p, s)
	} else if !os.IsNotExist(err) {
		return Status{}, err
	}
	// Base repository refs/pull/N/head also represent fork PRs. Detached HEADs
	// avoid checked-out branch conflicts; no checkout/reset touches existing trees.
	ref := fmt.Sprintf("refs/custom-visning/%s/pr-%d", id, p.Number)
	have := ""
	if commitPattern.MatchString(p.HeadOID) {
		have, _ = git(ctx, repo, "rev-parse", "--verify", p.HeadOID+"^{commit}")
	}
	if have == "" {
		if _, err := exec.LookPath("gh"); err != nil {
			return Status{}, fmt.Errorf("install GitHub CLI and run gh auth login before fetching PR commits: %w", err)
		}
		_, err = git(ctx, repo, "-c", "credential.helper=", "-c", "credential.helper=!gh auth git-credential", "fetch", "--no-tags", "--", "https://github.com/"+id+".git", fmt.Sprintf("+refs/pull/%d/head:%s", p.Number, ref))
		if err != nil {
			return Status{}, fmt.Errorf("PR #%d fetch failed (deleted PR, access, network or gh authentication): %w", p.Number, err)
		}
		have, err = git(ctx, repo, "rev-parse", ref+"^{commit}")
		if err != nil {
			return Status{}, err
		}
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
func (w *WorktreeManager) remember(p model.PullRequest, s Status) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.entries == nil {
		w.entries = map[string]Status{}
	}
	w.entries[key(p)] = s
	return writeJSON(filepath.Join(w.Repositories.Dir, "worktrees.json"), w.entries)
}
