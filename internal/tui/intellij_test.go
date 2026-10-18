package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/haakonph/custom-visning-terminal/internal/connectors/copilot"
)

func TestSessionProjectPreservesProjectAndFallsBack(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "monorepo project")
	cwd := filepath.Join(project, "src")
	if err := os.MkdirAll(cwd, 0700); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		session copilot.Session
		want    string
	}{
		{copilot.Session{ProjectRoot: project, Cwd: cwd, GitRoot: root}, project},
		{copilot.Session{ProjectRoot: filepath.Join(root, "missing"), Cwd: cwd, GitRoot: root}, cwd},
		{copilot.Session{ProjectRoot: "relative", Cwd: "https://example.test", GitRoot: root}, root},
	} {
		got, err := sessionProject(test.session)
		if err != nil || got != test.want {
			t.Fatalf("project: %q %v, want %q", got, err, test.want)
		}
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := sessionProject(copilot.Session{ProjectRoot: file}); err == nil {
		t.Fatal("opened a file as a project")
	}
	if _, err := sessionProject(copilot.Session{}); err == nil {
		t.Fatal("accepted missing paths")
	}
}
func TestIdeaCommandsKeepProjectAsSingleArgument(t *testing.T) {
	project := filepath.Join(t.TempDir(), "project with spaces; $(command)")
	for _, test := range []struct {
		platform, override, command string
		args                        []string
		wait                        bool
	}{
		{"darwin", "", "open", []string{"-a", "IntelliJ IDEA", project}, true},
		{"linux", "", "idea", []string{project}, false},
		{"windows", "", "idea64.exe", []string{project}, false},
		{"darwin", "/Toolbox with spaces/idea", "/Toolbox with spaces/idea", []string{project}, false},
	} {
		got := ideaCommand(test.platform, test.override, project)
		if got.command != test.command || !reflect.DeepEqual(got.args, test.args) || got.wait != test.wait {
			t.Fatalf("launch: %+v", got)
		}
	}
}
func TestIntelliJLauncherReceivesLiteralPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX launcher fixture")
	}
	root := t.TempDir()
	project := filepath.Join(root, "project spaces; $(touch injected)")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	launcher := filepath.Join(root, "idea launcher")
	output := filepath.Join(root, "arguments")
	if err := os.WriteFile(launcher, []byte("#!/bin/sh\nprintf '%s\\n' \"$#\" \"$1\" > \"$IDE_TEST_OUTPUT\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COPILOT_IDEA_COMMAND", launcher)
	t.Setenv("IDE_TEST_OUTPUT", output)
	if err := openIntelliJ(context.Background(), copilot.Session{ProjectRoot: project}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		b, err := os.ReadFile(output)
		if err == nil && string(b) == "1\n"+project+"\n" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("launcher arguments: %q %v", b, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Setenv("COPILOT_IDEA_COMMAND", filepath.Join(root, "missing-launcher"))
	if err := openIntelliJ(context.Background(), copilot.Session{ProjectRoot: project}); err == nil || !strings.Contains(err.Error(), "COPILOT_IDEA_COMMAND") {
		t.Fatalf("launcher failure: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := openIntelliJ(ctx, copilot.Session{ProjectRoot: project}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}
func TestCopilotOpenIDEUsesSelectedOrDetailedSession(t *testing.T) {
	m := ready(t).WithCopilot(testCopilot(t), true)
	m.copilot.sessions = []copilot.Session{{ID: "first"}, {ID: "selected"}}
	m.copilot.selected = 1
	var opened string
	m.copilot.openIDE = func(ctx context.Context, s copilot.Session) error { opened = s.ID; return nil }
	m, cmd := sequence(m, "ee")
	if cmd == nil || !m.copilot.openingIDE {
		t.Fatal("IDE action not scheduled")
	}
	_, duplicate := sequence(m, "ee")
	if duplicate != nil {
		t.Fatal("duplicate launch while pending")
	}
	m, _ = update(m, cmd())
	if opened != "selected" || m.copilot.openingIDE || m.copilot.ideNotice == "" {
		t.Fatal("wrong selection or missing feedback")
	}
	m.copilot.detail = &copilot.Detail{Session: copilot.Session{ID: "detail"}}
	m, cmd = sequence(m, "ee")
	m, _ = update(m, cmd())
	if opened != "detail" {
		t.Fatal("opened list row instead of detail")
	}
	m, _ = update(m, copilotOpenMsg{err: errors.New("launcher unavailable")})
	if m.copilot.ideNotice != "launcher unavailable" {
		t.Fatal("launch failure not shown")
	}
	m, _ = update(m, key('/'))
	m, _ = update(m, key('i'))
	if m.copilot.openingIDE || m.copilot.input.Value() != "i" {
		t.Fatal("filter key launched IDE")
	}
	m.copilot.filtering = false
	m.copilot.detail = nil
	m.copilot.sessions = nil
	_, cmd = sequence(m, "ee")
	if cmd != nil {
		t.Fatal("launched IDE without session")
	}
}
