package local

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoveWorktreeAndRecreate(t *testing.T) {
	w, p, _ := fixture(t)
	p.HeadRef = "feature"
	ctx := context.Background()
	s, err := w.Prepare(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	run(t, s.Path, "commit", "--allow-empty", "-m", "local commit")
	head := run(t, s.Path, "rev-parse", "HEAD")
	branch := run(t, s.Path, "symbolic-ref", "--short", "HEAD")
	physical, err := common(ctx, s.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Remove(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.Path); !os.IsNotExist(err) {
		t.Fatal("checkout remains", err)
	}
	if strings.Contains(run(t, physical, "worktree", "list", "--porcelain"), s.Path) {
		t.Fatal("Git registration remains")
	}
	if run(t, physical, "rev-parse", branch) != head {
		t.Fatal("local commits lost")
	}
	loaded, err := New(w.Repositories.Dir, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.entries[key(p)]; ok {
		t.Fatal("persisted entry remains")
	}
	// The fallback branch permits a fresh checkout while retaining local commits.
	fresh, err := loaded.Prepare(ctx, p)
	if err != nil || fresh.Head != p.HeadOID {
		t.Fatal(fresh, err)
	}
}

func TestRemoveRepeatedlyRecreatesWithLocalCommits(t *testing.T) {
	w, p, _ := fixture(t)
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		s, err := w.Prepare(ctx, p)
		if err != nil || s.Head != p.HeadOID {
			t.Fatal(s, err)
		}
		run(t, s.Path, "commit", "--allow-empty", "-m", "preserve local work")
		head := run(t, s.Path, "rev-parse", "HEAD")
		branch := run(t, s.Path, "symbolic-ref", "--short", "HEAD")
		physical, err := common(ctx, s.Path)
		if err != nil {
			t.Fatal(err)
		}
		if err := w.Remove(ctx, p); err != nil {
			t.Fatal(err)
		}
		if run(t, physical, "rev-parse", branch) != head {
			t.Fatal("local branch changed")
		}
	}
}

func TestRemoveRefusesUnsafeWorktrees(t *testing.T) {
	for _, kind := range []string{"untracked", "tracked", "locked", "operation", "detached", "main", "foreign", "directory", "missing"} {
		t.Run(kind, func(t *testing.T) {
			w, p, source := fixture(t)
			ctx := context.Background()
			s, err := w.Prepare(ctx, p)
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "detached":
				run(t, s.Path, "checkout", "--detach")
				run(t, s.Path, "commit", "--allow-empty", "-m", "detached work")
			case "untracked", "tracked":
				file := filepath.Join(s.Path, "keep.txt")
				if err := os.WriteFile(file, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
				if kind == "tracked" {
					run(t, s.Path, "add", "keep.txt")
				}
			case "locked":
				run(t, s.Path, "worktree", "lock", s.Path)
			case "operation":
				marker := run(t, s.Path, "rev-parse", "--git-path", "BISECT_LOG")
				if !filepath.IsAbs(marker) {
					marker = filepath.Join(s.Path, marker)
				}
				if err := os.WriteFile(marker, []byte("active"), 0600); err != nil {
					t.Fatal(err)
				}
			case "main":
				run(t, source, "remote", "add", "origin", "https://github.com/acme/repo.git")
				s.Path = source
				if err := w.remember(p, s); err != nil {
					t.Fatal(err)
				}
			case "foreign":
				run(t, s.Path, "remote", "set-url", "origin", "https://github.com/other/repo.git")
			case "directory":
				s.Path = t.TempDir()
				if err := w.remember(p, s); err != nil {
					t.Fatal(err)
				}
			case "missing":
				p.Number = 1234
			}
			if err := w.Remove(ctx, p); err == nil {
				t.Fatal("unsafe removal allowed")
			}
			if _, err := os.Stat(s.Path); err != nil {
				t.Fatal("checkout changed", err)
			}
		})
	}
}
