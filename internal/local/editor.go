package local

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type EditorLauncher struct{ Editors Editors }

func (l EditorLauncher) Command(name, path string) (*exec.Cmd, error) {
	if name == "" {
		name = l.Editors.Default
	}
	var e Editor
	switch name {
	case "intellij":
		e = l.Editors.IntelliJ
	case "vscode":
		e = l.Editors.VSCode
	case "custom":
		e = l.Editors.Custom
	default:
		return nil, fmt.Errorf("unknown editor %q; configure intellij, vscode or custom", name)
	}
	binary, err := exec.LookPath(e.Command)
	if err != nil {
		return nil, fmt.Errorf("editor %q unavailable: install %q or change editors in config.json", name, e.Command)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	args := make([]string, len(e.Args))
	found := false
	for i, arg := range e.Args {
		found = found || strings.Contains(arg, "{path}")
		args[i] = strings.ReplaceAll(arg, "{path}", path)
	}
	if !found {
		args = append(args, path)
	}
	return exec.Command(binary, args...), nil
}
func (l EditorLauncher) Launch(name, path string) error {
	cmd, err := l.Command(name, path)
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return fmt.Errorf("start editor: %w", err)
	}
	// Reap the process without tying its lifetime to the TUI context.
	go func() { _ = cmd.Wait() }()
	return nil
}
