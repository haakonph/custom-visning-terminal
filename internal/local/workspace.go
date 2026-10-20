package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Warm prepares one main checkout per repository, sharing objects with PRs.
// It never checks out or pulls in a user's registered working directory.
func (w *WorktreeManager) Warm(ctx context.Context, id string) error {
	id = strings.ToLower(id)
	if !validIdentity(id) {
		return fmt.Errorf("invalid repository identity")
	}
	unlock := w.Repositories.lock(id)
	defer unlock()
	repo, err := w.Repositories.prepare(ctx, id)
	if err != nil {
		return err
	}
	physical, err := common(ctx, repo)
	if err != nil {
		return err
	}
	unlockPhysical := w.Repositories.lock("path:" + physical)
	defer unlockPhysical()
	branch := w.Workspace.MainBranch
	if _, err = git(ctx, repo, "check-ref-format", "refs/heads/"+branch); err != nil {
		return fmt.Errorf("invalid workspace.main_branch")
	}
	ref := "refs/custom-visning/main/" + id
	if _, err = git(ctx, repo, "-c", "credential.helper=", "-c", "credential.helper=!gh auth git-credential", "fetch", "--no-tags", "--", "https://github.com/"+id+".git", "+refs/heads/"+branch+":"+ref); err != nil {
		return err
	}
	head, err := git(ctx, repo, "rev-parse", ref+"^{commit}")
	if err != nil {
		return err
	}
	path := filepath.Join(w.Repositories.Dir, "main", filepath.FromSlash(id))
	if _, err = os.Lstat(path); os.IsNotExist(err) {
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		// A registered repo may already have main checked out. Give our separate
		// warm checkout its own branch rather than disturbing that checkout.
		if _, err = git(ctx, repo, "worktree", "add", "--", path, branch); err != nil {
			if _, err = git(ctx, repo, "worktree", "add", "-b", "custom-visning/warm-"+branch, "--", path, head); err != nil {
				return err
			}
		}
	} else if err != nil {
		return err
	}
	s, err := inspect(ctx, path, repo, head)
	if err != nil {
		return err
	}
	if s.Modified {
		return fmt.Errorf("main checkout has local changes; left untouched: %s", path)
	}
	checkedBranch, err := git(ctx, path, "symbolic-ref", "--short", "HEAD")
	if err != nil || (checkedBranch != branch && checkedBranch != "custom-visning/warm-"+branch) {
		return fmt.Errorf("main checkout is on another branch; left untouched: %s", path)
	}
	if err = checkGitOperation(ctx, path); err != nil {
		return err
	}
	if s.Outdated {
		if _, err = git(ctx, path, "merge-base", "--is-ancestor", s.Head, head); err != nil {
			return fmt.Errorf("main has local commits or rewritten history; left untouched: %s", path)
		}
		if _, err = git(ctx, path, "merge", "--ff-only", "--no-autostash", "--no-edit", "--no-overwrite-ignore", head); err != nil {
			return fmt.Errorf("main cannot fast-forward; checkout preserved: %w", err)
		}
	}
	return w.prepareProject(ctx, path, head)
}

func checkGitOperation(ctx context.Context, path string) error {
	for _, name := range []string{"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "BISECT_LOG", "BISECT_START", "rebase-merge", "rebase-apply", "sequencer"} {
		marker, err := git(ctx, path, "rev-parse", "--git-path", name)
		if err != nil {
			return err
		}
		if !filepath.IsAbs(marker) {
			marker = filepath.Join(path, marker)
		}
		if _, err = os.Lstat(marker); !os.IsNotExist(err) {
			return fmt.Errorf("Git operation in progress; update manually")
		}
	}
	return nil
}

type projectCommand struct {
	command string
	args    []string
}

func projectCommands(path string, mavenFlags []string) []projectCommand {
	has := func(name string) bool { s, err := os.Stat(filepath.Join(path, name)); return err == nil && !s.IsDir() }
	var result []projectCommand
	if has("pom.xml") {
		command := "mvn"
		if has("mvnw") {
			command = filepath.Join(path, "mvnw")
		}
		args := append([]string{"-B", "-DskipTests"}, mavenFlags...)
		result = append(result, projectCommand{command, append(args, "install")})
	} else if has("build.gradle") || has("build.gradle.kts") || has("settings.gradle") || has("settings.gradle.kts") {
		command := "gradle"
		if has("gradlew") {
			command = filepath.Join(path, "gradlew")
		}
		result = append(result, projectCommand{command, []string{"--no-daemon", "assemble", "-x", "test"}})
	}
	if has("package.json") {
		args := []string{"install", "--no-audit", "--no-fund"}
		if has("package-lock.json") || has("npm-shrinkwrap.json") {
			args[0] = "ci"
		}
		result = append(result, projectCommand{"npm", args})
	}
	return result
}

// A successful preparation is cached per checkout, commit and configuration.
// Separate worktrees retain their own build output and node_modules.
func (w *WorktreeManager) prepareProject(ctx context.Context, path, head string) error {
	config, err := json.Marshal(w.Workspace)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(append([]byte(path+"\x00"+head+"\x00"), config...))
	name := hex.EncodeToString(digest[:])
	stamp, err := git(ctx, path, "rev-parse", "--git-path", "custom-visning-prepared")
	if err != nil {
		return err
	}
	if !filepath.IsAbs(stamp) {
		stamp = filepath.Join(path, stamp)
	}
	if previous, readErr := os.ReadFile(stamp); readErr == nil && string(previous) == name {
		return nil
	}
	var commands []projectCommand
	if w.Workspace.Install {
		commands = projectCommands(path, w.Workspace.MavenFlags)
	}
	if indexer := w.Workspace.Indexer; indexer.Command != "" {
		args := make([]string, 0, len(indexer.Args)+1)
		replaced := false
		for _, arg := range indexer.Args {
			replaced = replaced || strings.Contains(arg, "{path}")
			args = append(args, strings.ReplaceAll(arg, "{path}", path))
		}
		if !replaced {
			args = append(args, path)
		}
		commands = append(commands, projectCommand{indexer.Command, args})
	}
	logPath := filepath.Join(w.Repositories.Dir, "logs", name+".log")
	if err = os.MkdirAll(filepath.Dir(logPath), 0700); err != nil {
		return err
	}
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer log.Close()
	for _, command := range commands {
		cmd := exec.CommandContext(ctx, command.command, command.args...)
		cmd.Dir, cmd.Stdout, cmd.Stderr = path, log, log
		cmd.Env = append(os.Environ(), "CI=true", "GIT_TERMINAL_PROMPT=0")
		if err = cmd.Run(); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("%s preparation failed; see %s: %w", filepath.Base(command.command), logPath, err)
		}
	}
	if err = os.MkdirAll(filepath.Dir(stamp), 0700); err != nil {
		return err
	}
	return os.WriteFile(stamp, []byte(name), 0600)
}

func (w *WorktreeManager) finish(ctx context.Context, s Status, err error) (Status, error) {
	if err != nil {
		return s, err
	}
	if !s.Modified && !s.Outdated && s.Note == "" {
		if err = w.prepareProject(ctx, s.Path, s.Head); err != nil {
			if ctx.Err() != nil {
				return s, ctx.Err()
			}
			s.Note = err.Error()
		}
		current, inspectErr := inspect(ctx, s.Path, s.Path, s.Head)
		if inspectErr != nil {
			return s, inspectErr
		}
		current.Note = s.Note
		s = current
	}
	if w.Workspace.Indexer.Command == "" && s.Note == "" {
		s.Note = "headless indexing not configured"
	}
	return s, nil
}
