package tui

import (
	"context"
	"errors"
	appconfig "github.com/haakonph/custom-visning-terminal/internal/config"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/haakonph/custom-visning-terminal/internal/connectors/copilot"
)

type ideaLaunch struct {
	command string
	args    []string
	wait    bool
}

// Prefer an explicit project root, keeping worktrees and monorepo projects separate.
func sessionProject(session copilot.Session) (string, error) {
	for _, path := range []string{session.ProjectRoot, session.Cwd, session.GitRoot} {
		if !filepath.IsAbs(path) || strings.ContainsRune(path, 0) {
			continue
		}
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			return filepath.Clean(path), nil
		}
	}
	return "", errors.New("Sesjonen har ingen tilgjengelig lokal prosjektmappe eller arbeidskatalog.")
}

func ideaCommand(platform, override, project string) ideaLaunch {
	if override != "" {
		return ideaLaunch{command: override, args: []string{project}}
	}
	switch platform {
	case "darwin":
		return ideaLaunch{command: "open", args: []string{"-a", "IntelliJ IDEA", project}, wait: true}
	case "windows":
		return ideaLaunch{command: "idea64.exe", args: []string{project}}
	default:
		return ideaLaunch{command: "idea", args: []string{project}}
	}
}

func openIntelliJ(ctx context.Context, session copilot.Session) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	project, err := sessionProject(session)
	if err != nil {
		return err
	}
	var settings struct {
		Command string `json:"intellij_command"`
	}
	if err := appconfig.Section(appconfig.Path(), "copilot", &settings); err != nil {
		return err
	}
	if override := os.Getenv("COPILOT_IDEA_COMMAND"); override != "" {
		settings.Command = override
	}
	launch := ideaCommand(runtime.GOOS, settings.Command, project)
	if launch.wait {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if exec.CommandContext(ctx, launch.command, launch.args...).Run() != nil {
			return errors.New("Kunne ikke åpne IntelliJ IDEA. Sett COPILOT_IDEA_COMMAND til IntelliJ-launcheren.")
		}
		return nil
	}
	// An IDE process may stay alive for hours. Reap it without blocking the UI or
	// terminating the user's IDE when the terminal app closes. No shell is used.
	command := exec.Command(launch.command, launch.args...)
	if err := command.Start(); err != nil {
		return errors.New("Fant ikke eller kunne ikke starte IntelliJ-launcheren. Legg idea på PATH eller sett COPILOT_IDEA_COMMAND til launcherens filsti.")
	}
	go func() { _ = command.Wait() }()
	return nil
}
