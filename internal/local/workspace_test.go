package local

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMavenFlagsConfig(t *testing.T) {
	for _, tc := range []struct {
		name, config string
		want         []string
	}{
		{"existing", `{"workspace":{"install":true}}`, []string{"-Dmaven.gitcommitid.skip=true"}},
		{"custom", `{"workspace":{"maven_flags":["-U","-Pdev"]}}`, []string{"-U", "-Pdev"}},
		{"empty", `{"workspace":{"maven_flags":[]}}`, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(tc.config), 0600); err != nil {
				t.Fatal(err)
			}
			config, err := LoadConfig(path)
			if err != nil || !reflect.DeepEqual(config.Workspace.MavenFlags, tc.want) {
				t.Fatal(config, err)
			}
		})
	}
}

func TestMavenFlagsReachBuildAndInvalidateCache(t *testing.T) {
	w, p, _ := fixture(t)
	s, err := w.Prepare(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	w.Workspace.Install = true
	w.Workspace.MavenFlags = Defaults().Workspace.MavenFlags
	for name, content := range map[string]string{
		"pom.xml": "<project/>",
		"mvnw":    "#!/bin/sh\nprintf '%s\\n' \"$@\" >> maven-runs\n",
	} {
		if err := os.WriteFile(filepath.Join(s.Path, name), []byte(content), 0700); err != nil {
			t.Fatal(err)
		}
	}
	prepare := func() {
		t.Helper()
		if err := w.prepareProject(context.Background(), s.Path, s.Head); err != nil {
			t.Fatal(err)
		}
	}
	prepare()
	prepare()
	runs, err := os.ReadFile(filepath.Join(s.Path, "maven-runs"))
	if err != nil || string(runs) != "-B\n-DskipTests\n-Dmaven.gitcommitid.skip=true\ninstall\n" {
		t.Fatalf("%s: %v", runs, err)
	}
	w.Workspace.MavenFlags = []string{"-U", "-Dlabel=two words; $(touch unexpected)"}
	prepare()
	runs, err = os.ReadFile(filepath.Join(s.Path, "maven-runs"))
	if err != nil || !strings.HasSuffix(string(runs), "-B\n-DskipTests\n-U\n-Dlabel=two words; $(touch unexpected)\ninstall\n") || strings.Count(string(runs), "install\n") != 2 {
		t.Fatalf("%s: %v", runs, err)
	}
	if _, err := os.Stat(filepath.Join(s.Path, "unexpected")); !os.IsNotExist(err) {
		t.Fatal("shell interpreted Maven flag", err)
	}
	// Explicit empty flags survive a JSON round trip.
	w.Workspace.MavenFlags = []string{}
	data, _ := json.Marshal(w.Workspace)
	if !strings.Contains(string(data), `"maven_flags":[]`) {
		t.Fatal(string(data))
	}
	prepare()
	runs, err = os.ReadFile(filepath.Join(s.Path, "maven-runs"))
	if err != nil || !strings.HasSuffix(string(runs), "-B\n-DskipTests\ninstall\n") || strings.Count(string(runs), "install\n") != 3 {
		t.Fatalf("%s: %v", runs, err)
	}
}

func TestPRNameBranchAndTitleChange(t *testing.T) {
	w, p, _ := fixture(t)
	p.Title, p.HeadRef = "Fix login / ../ bug 🚀", "feature/login"
	s, err := w.Prepare(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(s.Path) != "pr-1-fix-login-bug" {
		t.Fatal(s.Path)
	}
	if branch := run(t, s.Path, "symbolic-ref", "--short", "HEAD"); branch != p.HeadRef {
		t.Fatal(branch)
	}
	p.Title = "Renamed PR"
	again, err := w.Prepare(context.Background(), p)
	if err != nil || again.Path != s.Path {
		t.Fatal(again, err)
	}
}

func TestBusyPRBranchGetsIndependentBranch(t *testing.T) {
	w, p, source := fixture(t)
	p.HeadRef = run(t, source, "symbolic-ref", "--short", "HEAD")
	run(t, source, "remote", "add", "origin", "https://github.com/acme/repo.git")
	w.Repositories.Paths = []string{source}
	s, err := w.Prepare(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if branch := run(t, s.Path, "symbolic-ref", "--short", "HEAD"); branch != "custom-visning/pr-1" {
		t.Fatal(branch)
	}
	if branch := run(t, source, "symbolic-ref", "--short", "HEAD"); branch != p.HeadRef {
		t.Fatal("source branch changed")
	}
}

func TestLegacyDetachedCheckoutGetsBranch(t *testing.T) {
	w, p, _ := fixture(t)
	s, err := w.Prepare(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	run(t, s.Path, "checkout", "--detach")
	run(t, s.Path, "branch", "-D", "custom-visning/pr-1")
	p.HeadRef = "feature/migrate"
	again, err := w.Prepare(context.Background(), p)
	if err != nil || again.Head != s.Head || again.Path != s.Path {
		t.Fatal(again, err)
	}
	if branch := run(t, s.Path, "symbolic-ref", "--short", "HEAD"); branch != p.HeadRef {
		t.Fatal(branch)
	}
}

func TestWarmMainUpdatesAndPreservesChanges(t *testing.T) {
	w, p, source := fixture(t)
	run(t, source, "branch", "-M", "main")
	ctx := context.Background()
	if err := w.Warm(ctx, p.Repository); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(w.Repositories.Dir, "main", "acme", "repo")
	if branch := run(t, path, "symbolic-ref", "--short", "HEAD"); branch != "main" {
		t.Fatal(branch)
	}
	p = advancePR(t, source, p)
	if err := w.Warm(ctx, p.Repository); err != nil {
		t.Fatal(err)
	}
	if head := run(t, path, "rev-parse", "HEAD"); head != p.HeadOID {
		t.Fatal(head)
	}
	if err := os.WriteFile(filepath.Join(path, "local.txt"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	p = advancePR(t, source, p)
	if err := w.Warm(ctx, p.Repository); err == nil || !strings.Contains(err.Error(), "local changes") {
		t.Fatal(err)
	}
	if head := run(t, path, "rev-parse", "HEAD"); head == p.HeadOID {
		t.Fatal("dirty main was updated")
	}
}

func TestWarmNeverChangesRegisteredCheckout(t *testing.T) {
	w, p, source := fixture(t)
	run(t, source, "branch", "-M", "main")
	run(t, source, "remote", "add", "origin", "https://github.com/acme/repo.git")
	w.Repositories.Paths = []string{source}
	if err := os.WriteFile(filepath.Join(source, "local.txt"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := w.Warm(context.Background(), p.Repository); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(w.Repositories.Dir, "main", "acme", "repo")
	if branch := run(t, path, "symbolic-ref", "--short", "HEAD"); branch != "custom-visning/warm-main" {
		t.Fatal(branch)
	}
	if got := run(t, source, "status", "--porcelain"); !strings.Contains(got, "local.txt") {
		t.Fatal(got)
	}
}

func TestProjectCommandsUseWrappersAndLockfile(t *testing.T) {
	path := t.TempDir()
	for _, name := range []string{"pom.xml", "mvnw", "package.json", "package-lock.json"} {
		if err := os.WriteFile(filepath.Join(path, name), nil, 0700); err != nil {
			t.Fatal(err)
		}
	}
	want := []projectCommand{
		{filepath.Join(path, "mvnw"), []string{"-B", "-DskipTests", "install"}},
		{"npm", []string{"ci", "--no-audit", "--no-fund"}},
	}
	if got := projectCommands(path, nil); !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
	if err := os.Remove(filepath.Join(path, "pom.xml")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"settings.gradle.kts", "gradlew"} {
		if err := os.WriteFile(filepath.Join(path, name), nil, 0700); err != nil {
			t.Fatal(err)
		}
	}
	want[0] = projectCommand{filepath.Join(path, "gradlew"), []string{"--no-daemon", "assemble", "-x", "test"}}
	if got := projectCommands(path, []string{"-Pcustom"}); !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
}

func TestPreparationRunsOncePerCommitAndFailureStillOpens(t *testing.T) {
	w, p, source := fixture(t)
	indexer := filepath.Join(t.TempDir(), "indexer")
	if err := os.WriteFile(indexer, []byte("#!/bin/sh\nprintf 'run\\n' >> \"$1/index-runs\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	w.Workspace.Indexer = Editor{Command: indexer, Args: []string{"{path}"}}
	s, err := w.Prepare(context.Background(), p)
	if err != nil || s.Note != "" {
		t.Fatal(s, err)
	}
	// The indexer log is untracked. Ignore it to model actual IDE/build caches.
	exclude := run(t, s.Path, "rev-parse", "--git-path", "info/exclude")
	if !filepath.IsAbs(exclude) {
		exclude = filepath.Join(s.Path, exclude)
	}
	if err = os.MkdirAll(filepath.Dir(exclude), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(exclude, []byte("index-runs\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = w.Prepare(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	runs, _ := os.ReadFile(filepath.Join(s.Path, "index-runs"))
	if string(runs) != "run\n" {
		t.Fatalf("repeated preparation: %q", runs)
	}
	p = advancePR(t, source, p)
	if _, err = w.Prepare(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	runs, _ = os.ReadFile(filepath.Join(s.Path, "index-runs"))
	if string(runs) != "run\nrun\n" {
		t.Fatalf("new commit not prepared: %q", runs)
	}
	w.Workspace.Indexer.Command = "custom-visning-missing-indexer"
	s, err = w.Prepare(context.Background(), p)
	if err != nil || s.Path == "" || !strings.Contains(s.Note, "preparation failed") {
		t.Fatal(s, err)
	}
	// Failures are retried rather than stamped as successful.
	s, err = w.Prepare(context.Background(), p)
	if err != nil || !strings.Contains(s.Note, "preparation failed") {
		t.Fatal(s, err)
	}
}
