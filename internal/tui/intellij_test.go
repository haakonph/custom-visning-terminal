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

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/copilot"
	"github.com/haakonph/custom-visning-terminal/internal/model"
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
	project := t.TempDir()
	m.copilot.sessions = []copilot.Session{{ID: "first", ProjectRoot: project}, {ID: "selected", ProjectRoot: project}}
	m.copilot.selected = 1
	var opened string
	m.copilot.openIDE = func(ctx context.Context, s copilot.Session) error { opened = s.ID; return nil }
	m, cmd := sequence(m, "eee")
	if cmd == nil || !m.copilot.openingIDE {
		t.Fatal("IDE action not scheduled")
	}
	_, duplicate := sequence(m, "eee")
	if duplicate != nil {
		t.Fatal("duplicate launch while pending")
	}
	m, _ = update(m, cmd())
	if opened != "selected" || m.copilot.openingIDE || m.copilot.ideNotice == "" {
		t.Fatal("wrong selection or missing feedback")
	}
	m.copilot.detail = &copilot.Detail{Session: copilot.Session{ID: "detail", ProjectRoot: project}}
	m, cmd = sequence(m, "eee")
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
	_, cmd = sequence(m, "eee")
	if cmd != nil {
		t.Fatal("launched IDE without session")
	}
}

func TestAIEditorShortcutsChooseDirectoryAndKeepOriginalTargets(t *testing.T) {
	project, chat, tree := t.TempDir(), t.TempDir(), t.TempDir()
	for _, target := range []string{"e", "w"} {
		t.Run(target, func(t *testing.T) {
			m := ready(t).WithCopilot(testCopilot(t), true)
			m.copilot.sessions = []copilot.Session{
				{ID: "first", ProjectRoot: project, Cwd: chat, GitRoot: tree},
				{ID: "other", ProjectRoot: project, Cwd: project, GitRoot: project},
			}
			var opened []copilot.Session
			m.copilot.openIDE = func(_ context.Context, s copilot.Session) error {
				opened = append(opened, s)
				return nil
			}
			m, cmd := sequence(m, "ee")
			if cmd != nil || m.sequence != "ee" || m.copilot.openingIDE || !strings.Contains(m.View().Content, "w worktree") {
				t.Fatal("ee must wait and show the two targets")
			}
			m.copilot.selected = 1 // Background/cursor changes must not retarget the sequence.
			m, cmd = sequence(m, target)
			if cmd == nil || m.sequence != "" || len(m.sequenceRefs) != 0 {
				t.Fatal("third key did not complete the sequence")
			}
			m, _ = update(m, cmd())
			want := chat
			if target == "w" {
				want = tree
			}
			if len(opened) != 1 || opened[0].ID != "first" || opened[0].ProjectRoot != want || m.copilot.openingIDE {
				t.Fatalf("opened %v, want first session at %s", opened, want)
			}
		})
	}
}

func TestAIEditorPrefixCancelInvalidAndMissingPaths(t *testing.T) {
	for _, finish := range []tea.KeyPressMsg{{Code: tea.KeyEscape}, key('x')} {
		m := ready(t).WithCopilot(testCopilot(t), true)
		m.copilot.sessions = []copilot.Session{{ID: "first"}}
		m, _ = sequence(m, "ee")
		m, cmd := update(m, finish)
		if cmd != nil || m.sequence != "" || len(m.sequenceRefs) != 0 || m.copilot.openingIDE {
			t.Fatal("canceled/invalid sequence launched an editor or retained targets")
		}
	}
	m := ready(t).WithCopilot(testCopilot(t), true)
	m.copilot.sessions = []copilot.Session{{ID: "missing", Cwd: "relative"}}
	m.copilot.openIDE = func(context.Context, copilot.Session) error {
		t.Fatal("launched IDE without an available local directory")
		return nil
	}
	m, cmd := sequence(m, "eew")
	if cmd == nil {
		t.Fatal("missing path validation command")
	}
	m, _ = update(m, cmd())
	if m.copilot.openingIDE || !strings.Contains(m.copilot.ideNotice, "ingen tilgjengelig lokal") {
		t.Fatal("missing directory did not produce visible feedback")
	}
}

func TestSessionEditorTargetFallsBackForIncompleteMetadata(t *testing.T) {
	project, chat, tree := t.TempDir(), t.TempDir(), t.TempDir()
	for _, tc := range []struct {
		session      copilot.Session
		target, want string
	}{
		{copilot.Session{ProjectRoot: project, Cwd: chat, GitRoot: tree}, "e", chat},
		{copilot.Session{ProjectRoot: project, Cwd: chat, GitRoot: tree}, "w", tree},
		{copilot.Session{ProjectRoot: project, Cwd: "/missing/chat", GitRoot: tree}, "e", project},
		{copilot.Session{ProjectRoot: project, Cwd: chat}, "w", project},
		{copilot.Session{Cwd: chat}, "w", chat},
		{copilot.Session{GitRoot: tree}, "e", tree},
	} {
		got, err := sessionEditorTarget(tc.session, tc.target)
		if err != nil || got.ProjectRoot != tc.want || got.Cwd != tc.session.Cwd || got.GitRoot != tc.session.GitRoot {
			t.Fatalf("target %s: got %+v, %v; want %s", tc.target, got, err, tc.want)
		}
	}
}

func TestAIEditorWorktreeShortcutUsesSelectionAndDetails(t *testing.T) {
	for _, details := range []bool{false, true} {
		m := ready(t).WithCopilot(testCopilot(t), true)
		first, second := t.TempDir(), t.TempDir()
		m.copilot.sessions = []copilot.Session{{ID: "first", GitRoot: first}, {ID: "second", GitRoot: second}}
		want := []string{first, second}
		if details {
			m.copilot.detail = &copilot.Detail{Session: copilot.Session{ID: "detail", GitRoot: first}}
			want = []string{first}
		} else {
			m, _ = sequence(m, " j ") // Mark both sessions.
		}
		var opened []string
		m.copilot.openIDE = func(_ context.Context, s copilot.Session) error {
			opened = append(opened, s.ProjectRoot)
			return nil
		}
		m, cmd := sequence(m, "eew")
		if cmd == nil {
			t.Fatal("worktree launch not scheduled")
		}
		m, _ = update(m, cmd())
		if !reflect.DeepEqual(opened, want) || m.copilot.sessions[0].ProjectRoot != "" {
			t.Fatalf("opened %v, want %v; stored metadata must remain intact", opened, want)
		}
	}
}

func TestRelatedAIEditorActionsInOtherViewsRemainImmediate(t *testing.T) {
	for _, page := range []string{"g", "t", "c", "o"} {
		m := unifiedModel(t)
		m, _ = sequence(m, "v"+page)
		m.entities.data.Links = append(m.entities.data.Links, model.EntityLink{
			Left: m.currentReferences()[0], Right: copilotRef(m.copilot.sessions[0]),
		})
		var opened []string
		m.copilot.openIDE = func(_ context.Context, s copilot.Session) error {
			opened = append(opened, s.ID)
			return nil
		}
		m, cmd := sequence(m, "eae")
		if cmd == nil || m.sequence != "" {
			t.Fatalf("%s: related AI editor action now requires a third editor key", page)
		}
		m, _ = update(m, cmd())
		if !reflect.DeepEqual(opened, []string{"ai1"}) {
			t.Fatalf("%s: opened %v, want the related session", page, opened)
		}
	}
}
