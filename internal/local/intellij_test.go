package local

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

type closureFunc func(context.Context, model.PullRequest) (bool, error)

func (f closureFunc) Closed(ctx context.Context, p model.PullRequest) (bool, error) { return f(ctx, p) }

func TestPruneIntelliJHistoryPreservesOtherProjects(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "worktrees", "pr-1")
	macro := "$USER_HOME$/worktrees/pr-1"
	data := []byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<application><!-- preserve formatting &amp; comments -->
 <component name="RecentProjectsManager">
  <option name="additionalInfo"><map>
   <entry key="%s"><value><RecentProjectMetaInfo><option name="opened" value="false" /></RecentProjectMetaInfo></value></entry>
   <entry key="%s-other"><value /></entry>
   <entry key="/normal/repo"><value /></entry>
  </map></option>
  <option name="recentPaths"><list><option value="%s" /><option value="/normal/repo" /></list></option>
  <option name="groups"><list><ProjectGroup><option name="projects"><list><option value="%s" /></list></option></ProjectGroup></list></option>
 </component>
 <component name="Other"><option value="%s" /></component>
</application>`, macro, path, path, macro, path))
	out, err := pruneIntelliJHistory(data, home, map[string]bool{path: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, removed := range []string{`<entry key="` + macro + `">`, `<option value="` + macro + `"`} {
		if strings.Contains(string(out), removed) {
			t.Fatalf("retained %s", removed)
		}
	}
	if strings.Count(string(out), `<option value="`+path+`"`) != 1 || !bytes.Contains(out, []byte(path+"-other")) || !bytes.Contains(out, []byte("<!-- preserve formatting &amp; comments -->")) {
		t.Fatalf("unrelated XML changed: %s", out)
	}
	again, err := pruneIntelliJHistory(out, home, map[string]bool{path: true})
	if err != nil || !bytes.Equal(out, again) {
		t.Fatal("not idempotent", err)
	}
}

func TestCleanupIntelliJChecksTrackedPRsAndKeepsWorktrees(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("APPDATA", filepath.Join(home, "config"))
	config, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(config, "JetBrains")
	// macOS does not use UserConfigDir for modern IntelliJ settings.
	if strings.Contains(config, "Library") {
		root = filepath.Join(home, "Library", "Application Support", "JetBrains")
	}
	file := filepath.Join(root, "IntelliJIdea2026.2", "options", "recentProjects.xml")
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		t.Fatal(err)
	}
	w, err := New(filepath.Join(home, "data"), Defaults())
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	var xml strings.Builder
	xml.WriteString(`<application><component name="RecentProjectsManager"><option name="additionalInfo"><map>`)
	for n := 1; n <= 4; n++ {
		p := model.PullRequest{Repository: "acme/repo", Number: n}
		path := w.path(p)
		paths = append(paths, path)
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "local.txt"), []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
		if n != 4 { // An untracked path must never be a cleanup candidate.
			if err := w.remember(p, Status{Path: path}); err != nil {
				t.Fatal(err)
			}
		}
		fmt.Fprintf(&xml, `<entry key="%s"><value /></entry>`, filepath.ToSlash(path))
	}
	xml.WriteString(`</map></option></component></application>`)
	if err := os.WriteFile(file, []byte(xml.String()), 0600); err != nil {
		t.Fatal(err)
	}
	// Restarting must still find old PRs, even when none are in the dashboard.
	w, err = New(w.Repositories.Dir, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	var checked []int
	err = w.CleanupIntelliJ(context.Background(), closureFunc(func(ctx context.Context, p model.PullRequest) (bool, error) {
		checked = append(checked, p.Number)
		if p.Number == 3 {
			return false, errors.New("offline")
		}
		return p.Number == 1, nil
	}))
	if err == nil || !strings.Contains(err.Error(), "offline") || len(checked) != 3 {
		t.Fatalf("checks: %v, %v", checked, err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	for i, path := range paths {
		if strings.Contains(string(data), filepath.ToSlash(path)) != (i != 0) {
			t.Fatalf("wrong project removed: %s", data)
		}
		if b, err := os.ReadFile(filepath.Join(path, "local.txt")); err != nil || string(b) != "keep" {
			t.Fatal("worktree changed", err)
		}
	}
	if len(w.entries) != 3 {
		t.Fatal("tracking removed")
	}
}

func TestMalformedIntelliJHistoryLeftUntouched(t *testing.T) {
	file := filepath.Join(t.TempDir(), "recentProjects.xml")
	data := []byte(`<application><component name="RecentProjectsManager"><entry key="/pr">`)
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := pruneIntelliJFile(file, "", map[string]bool{"/pr": true}); err == nil {
		t.Fatal("expected error")
	}
	got, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(data, got) {
		t.Fatal("malformed history changed", err)
	}
}

func TestIntelliJHistoryDiscovery(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, "config")
	for _, dir := range []string{
		filepath.Join(config, "JetBrains", "IdeaIC2025.1", "options"),
		filepath.Join(config, "JetBrains", "IntelliJIdea2026.2", "options"),
		filepath.Join(home, ".IdeaIC2019.3", "config", "options"),
		filepath.Join(config, "JetBrains", "PyCharm2026.2", "options"),
	} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "recentProjects.xml"), []byte(`<application />`), 0600); err != nil {
			t.Fatal(err)
		}
	}
	files, err := intellijHistoryFiles("linux", home, config)
	if err != nil || len(files) != 3 {
		t.Fatal(files, err)
	}
	for _, f := range files {
		if strings.Contains(f, "PyCharm") {
			t.Fatal(f)
		}
	}
	unchanged := []byte(`<application><component name="RecentProjectsManagerBase"><option value="/other" /></component></application>`)
	got, err := pruneIntelliJHistory(unchanged, home, map[string]bool{"/pr": true})
	if err != nil || !reflect.DeepEqual(got, unchanged) {
		t.Fatal("unchanged history rewritten", err)
	}
}
