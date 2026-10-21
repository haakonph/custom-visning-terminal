package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/haakonph/custom-visning-terminal/internal/storage"
)

type Folder = storage.Folder

func (w *WorktreeManager) folderStore() string {
	return filepath.Join(w.Repositories.Dir, "folders.sqlite")
}
func (w *WorktreeManager) SaveFolder(f Folder) error { return storage.SaveFolder(w.folderStore(), f) }

// Folders discovers only registered repositories and their Git worktrees, plus
// checkouts recorded by this app. It never scans arbitrary user directories.
func (w *WorktreeManager) Folders(ctx context.Context) ([]Folder, error) {
	saved, err := storage.LoadFolders(w.folderStore())
	if err != nil {
		return nil, err
	}
	folders := map[string]Folder{}
	for _, f := range saved {
		folders[f.Path] = f
	}
	w.Repositories.mu.Lock()
	repos := map[string]string{}
	for id, path := range w.Repositories.mappings {
		repos[path] = id
	}
	w.Repositories.mu.Unlock()
	for _, path := range w.Repositories.Paths {
		path, err = filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		repos[path] = ""
	}
	w.mu.Lock()
	for id, s := range w.entries {
		if s.Path != "" {
			f := folders[s.Path]
			f.Path = s.Path
			f.Repository = strings.SplitN(id, "#", 2)[0]
			folders[s.Path] = f
			repos[s.Path] = f.Repository
		}
	}
	w.mu.Unlock()
	for path, id := range repos {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if id == "" {
			raw, e := git(ctx, path, "remote", "get-url", "origin")
			if e == nil {
				id, _ = Identity(raw)
			}
		}
		output, e := git(ctx, path, "worktree", "list", "--porcelain", "-z")
		if e != nil { // Keep a registered folder visible even when Git metadata is missing.
			f := folders[path]
			f.Path, f.Repository = path, id
			folders[path] = f
			continue
		}
		var checkout string
		bare := false
		flush := func() {
			if checkout != "" && !bare {
				f := folders[checkout]
				f.Path, f.Repository = checkout, id
				folders[checkout] = f
			}
			checkout, bare = "", false
		}
		for _, field := range strings.Split(output, "\x00") {
			switch {
			case field == "":
				flush()
			case strings.HasPrefix(field, "worktree "):
				checkout = strings.TrimPrefix(field, "worktree ")
			case field == "bare":
				bare = true
			}
		}
		flush()
	}
	result := make([]Folder, 0, len(folders))
	for _, f := range folders {
		result = append(result, f)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Favorite != result[j].Favorite {
			return result[i].Favorite
		}
		return result[i].Path < result[j].Path
	})
	return result, nil
}

var issueFolderKey = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*-[0-9]+$`)

// CreateIssue creates a separate branch/worktree; existing paths and branches
// are never reset, overwritten or reused unless recorded for this issue/repo.
func (w *WorktreeManager) CreateIssue(ctx context.Context, id, issue, title string) (Folder, error) {
	id, issue = strings.ToLower(id), strings.ToUpper(issue)
	if !validIdentity(id) || !issueFolderKey.MatchString(issue) {
		return Folder{}, fmt.Errorf("invalid repository or Jira issue")
	}
	unlock := w.Repositories.lock(id)
	defer unlock()
	saved, err := storage.LoadFolders(w.folderStore())
	if err != nil {
		return Folder{}, err
	}
	repo, err := w.Repositories.prepare(ctx, id)
	if err != nil {
		return Folder{}, err
	}
	physical, err := common(ctx, repo)
	if err != nil {
		return Folder{}, err
	}
	unlockPhysical := w.Repositories.lock("path:" + physical)
	defer unlockPhysical()
	branch := "custom-visning/jira/" + strings.ToLower(issue)
	path := filepath.Join(w.Repositories.Dir, "jira", filepath.FromSlash(id), strings.ToLower(issue))
	for _, f := range saved {
		if f.Path == path && f.Repository == id && f.IssueKey == issue {
			found, e := common(ctx, path)
			current, be := git(ctx, path, "symbolic-ref", "--short", "HEAD")
			if e == nil && be == nil && found == physical && current == branch {
				return f, nil
			}
			return Folder{}, fmt.Errorf("registrert Jira-mappe mangler eller har endret repository/branch: %s", path)
		}
	}
	if _, e := os.Lstat(path); !os.IsNotExist(e) {
		return Folder{}, fmt.Errorf("mappen finnes allerede: %s", path)
	}
	base := w.Workspace.MainBranch
	if base == "" {
		base = "main"
	}
	if _, err := git(ctx, repo, "check-ref-format", "refs/heads/"+base); err != nil {
		return Folder{}, fmt.Errorf("invalid workspace.main_branch")
	}
	ref := "refs/custom-visning/jira-base/" + id
	if _, err = git(ctx, repo, "-c", "credential.helper=", "-c", "credential.helper=!gh auth git-credential", "fetch", "--no-tags", "--", "https://github.com/"+id+".git", "+refs/heads/"+base+":"+ref); err != nil {
		return Folder{}, err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return Folder{}, err
	}
	if _, err = git(ctx, repo, "worktree", "add", "-b", branch, "--", path, ref); err != nil {
		return Folder{}, err
	}
	f := Folder{Path: path, Repository: id, IssueKey: issue, Title: title}
	if err = w.SaveFolder(f); err != nil {
		return f, fmt.Errorf("mappen er opprettet på %s, men registrering feilet: %w", path, err)
	}
	return f, nil
}
