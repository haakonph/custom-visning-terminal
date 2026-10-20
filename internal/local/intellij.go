package local

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

type PRClosureSource interface {
	Closed(context.Context, model.PullRequest) (bool, error)
}

// CleanupIntelliJ only removes recent-project references to this app's managed
// PR worktrees. Keep the worktrees and tracking data, including for closed PRs:
// local edits remain available and IDEs may rewrite their history while running.
func (w *WorktreeManager) CleanupIntelliJ(ctx context.Context, source PRClosureSource) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	files, err := intellijHistoryFiles(runtime.GOOS, home, config)
	if err != nil || len(files) == 0 {
		return err
	}
	w.mu.Lock()
	entries := make(map[string]Status, len(w.entries))
	for id, status := range w.entries {
		entries[id] = status
	}
	w.mu.Unlock()
	paths := map[string]bool{}
	var problems []error
	for id, status := range entries {
		repo, number, ok := strings.Cut(id, "#")
		n, err := strconv.Atoi(number)
		// Never infer ownership from a directory name or touch registered repos.
		base := filepath.Join(w.Repositories.Dir, "worktrees", filepath.FromSlash(repo))
		rel, relErr := filepath.Rel(base, status.Path)
		if !ok || err != nil || !validIdentity(repo) || n <= 0 || !filepath.IsAbs(status.Path) || relErr != nil || rel == "." || strings.HasPrefix(rel, "..") || strings.ContainsRune(rel, filepath.Separator) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		closed, err := source.Closed(ctx, model.PullRequest{Repository: repo, Number: n})
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", id, err))
			continue
		}
		if closed {
			paths[filepath.Clean(status.Path)] = true
		}
	}
	if len(paths) > 0 {
		for _, file := range files {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := pruneIntelliJFile(file, home, paths); err != nil {
				problems = append(problems, fmt.Errorf("%s: %w", file, err))
			}
		}
	}
	return errors.Join(problems...)
}

func intellijHistoryFiles(platform, home, config string) ([]string, error) {
	root := filepath.Join(config, "JetBrains")
	if platform == "darwin" {
		root = filepath.Join(home, "Library", "Application Support", "JetBrains")
	}
	patterns := []string{
		filepath.Join(root, "IntelliJIdea*", "options", "recentProjects.xml"),
		filepath.Join(root, "IdeaIC*", "options", "recentProjects.xml"),
		filepath.Join(home, ".IntelliJIdea*", "config", "options", "recentProjects.xml"),
		filepath.Join(home, ".IdeaIC*", "config", "options", "recentProjects.xml"),
	}
	var files []string
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		files = append(files, matches...)
	}
	sort.Strings(files)
	return files, nil
}

// Remove complete XML elements using byte offsets, preserving all unrelated
// bytes (including comments, whitespace and IDE-version-specific settings).
func pruneIntelliJHistory(data []byte, home string, paths map[string]bool) ([]byte, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	type frame struct {
		start     int
		remove    bool
		component bool
	}
	var stack []frame
	type span struct{ start, end int }
	var spans []span
	for {
		start := int(d.InputOffset())
		token, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("invalid IntelliJ history XML: %w", err)
		}
		switch t := token.(type) {
		case xml.StartElement:
			inside := len(stack) > 0 && stack[len(stack)-1].component
			remove := false
			for _, attr := range t.Attr {
				if t.Name.Local == "component" && attr.Name.Local == "name" && (attr.Value == "RecentProjectsManager" || attr.Value == "RecentProjectsManagerBase") {
					inside = true
				}
				if inside && ((t.Name.Local == "entry" && attr.Name.Local == "key") || (t.Name.Local == "option" && attr.Name.Local == "value")) {
					path := attr.Value
					if strings.HasPrefix(path, "$USER_HOME$/") {
						path = filepath.Join(home, filepath.FromSlash(strings.TrimPrefix(path, "$USER_HOME$/")))
					}
					remove = remove || (filepath.IsAbs(path) && paths[filepath.Clean(path)])
				}
			}
			stack = append(stack, frame{start, remove, inside})
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, errors.New("invalid IntelliJ history XML")
			}
			f := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if f.remove {
				spans = append(spans, span{f.start, int(d.InputOffset())})
			}
		}
	}
	if len(stack) > 0 {
		return nil, errors.New("incomplete IntelliJ history XML")
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	var out bytes.Buffer
	pos := 0
	for _, s := range spans {
		if s.start < pos {
			continue // A removed parent already contains this child.
		}
		out.Write(data[pos:s.start])
		pos = s.end
	}
	out.Write(data[pos:])
	return out.Bytes(), nil
}

func pruneIntelliJFile(path, home string, paths map[string]bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("history is not a regular file; left untouched")
	}
	before, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	after, err := pruneIntelliJHistory(before, home, paths)
	if err != nil || bytes.Equal(before, after) {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".recentProjects-*.xml")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(info.Mode().Perm()); err == nil {
		_, err = f.Write(after)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	// Avoid overwriting a concurrent save by IntelliJ. A later refresh retries.
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, before) {
		return errors.New("IntelliJ updated its history; cleanup will retry")
	}
	return os.Rename(f.Name(), path)
}
