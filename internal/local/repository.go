package local

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/haakonph/custom-visning-terminal/internal/storage"
)

var identityPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func Identity(remote string) (string, error) {
	remote = strings.TrimSpace(remote)
	if strings.HasPrefix(remote, "git@github.com:") {
		remote = "ssh://git@github.com/" + strings.TrimPrefix(remote, "git@github.com:")
	}
	u, err := url.Parse(remote)
	if err != nil || !strings.EqualFold(u.Hostname(), "github.com") || (u.Scheme != "https" && u.Scheme != "ssh") || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("not a GitHub SSH/HTTPS remote")
	}
	id := strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), ".git")
	if !validIdentity(id) {
		return "", fmt.Errorf("invalid GitHub repository identity")
	}
	return strings.ToLower(id), nil
}
func validIdentity(id string) bool {
	if !identityPattern.MatchString(id) {
		return false
	}
	for _, p := range strings.Split(id, "/") {
		if p == "." || p == ".." || strings.HasPrefix(p, "-") {
			return false
		}
	}
	return true
}
func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	b, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("git %s failed; check repository access, gh auth status, network and worktree metadata (%w)", args[0], err)
	}
	return strings.TrimSpace(string(b)), nil
}

type RepositoryManager struct {
	Dir      string
	Paths    []string
	mu       sync.Mutex
	locks    map[string]*sync.Mutex
	mappings map[string]string
}

func (r *RepositoryManager) lock(id string) func() {
	r.mu.Lock()
	if r.locks == nil {
		r.locks = map[string]*sync.Mutex{}
	}
	l := r.locks[id]
	if l == nil {
		l = &sync.Mutex{}
		r.locks[id] = l
	}
	r.mu.Unlock()
	l.Lock()
	return l.Unlock
}
func (r *RepositoryManager) matches(ctx context.Context, path, id string) bool {
	remotes, err := git(ctx, path, "remote")
	if err != nil {
		return false
	}
	for _, name := range strings.Fields(remotes) {
		raw, err := git(ctx, path, "config", "--get", "remote."+name+".url")
		if err == nil {
			got, e := Identity(raw)
			if e == nil && got == id {
				return true
			}
		}
	}
	return false
}

// Prepare is serialized per identity. Paths are exact repositories, never scanned.
func (r *RepositoryManager) Prepare(ctx context.Context, id string) (string, error) {
	id = strings.ToLower(id)
	if !validIdentity(id) {
		return "", fmt.Errorf("invalid repository identity %q", id)
	}
	unlock := r.lock(id)
	defer unlock()
	return r.prepare(ctx, id)
}
func (r *RepositoryManager) prepare(ctx context.Context, id string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", fmt.Errorf("install Git and add it to PATH: %w", err)
	}
	for _, p := range r.Paths {
		p, err := filepath.Abs(p)
		if err == nil && r.matches(ctx, p, id) {
			return p, r.remember(id, p)
		}
	}
	r.mu.Lock()
	cached := r.mappings[id]
	r.mu.Unlock()
	expected := filepath.Join(r.Dir, "repos", filepath.FromSlash(id)+".git")
	for _, p := range []string{cached, expected} {
		if p != "" && r.matches(ctx, p, id) {
			return p, r.remember(id, p)
		}
	}
	if _, err := os.Lstat(expected); !os.IsNotExist(err) {
		return "", fmt.Errorf("cache path %s exists but is not the matching repository; move it manually", expected)
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return "", fmt.Errorf("install GitHub CLI and run gh auth login: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(expected), 0700); err != nil {
		return "", err
	}
	// Stage separately: cancellation/failure never leaves a seemingly reusable clone.
	stage, err := os.MkdirTemp(filepath.Dir(expected), ".clone-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	if _, err = git(ctx, filepath.Dir(expected), "-c", "credential.helper=", "-c", "credential.helper=!gh auth git-credential", "clone", "--bare", "--", "https://github.com/"+id+".git", stage); err != nil {
		return "", err
	}
	if err = os.Rename(stage, expected); err != nil {
		return "", err
	}
	return expected, r.remember(id, expected)
}
func (r *RepositoryManager) remember(id, path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.mappings == nil {
		r.mappings = map[string]string{}
	}
	r.mappings[id] = path
	return storage.Save(filepath.Join(r.Dir, "repositories.json"), r.mappings)
}
