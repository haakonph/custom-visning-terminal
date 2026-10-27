package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/haakonph/custom-visning-terminal/internal/pullrequests/github"
	"github.com/haakonph/custom-visning-terminal/internal/updater"
)

func runUpdate(ctx context.Context, download bool, input io.Reader, output io.Writer) error {
	token, err := github.UpdateToken(ctx)
	if err != nil {
		return fmt.Errorf("update authentication missing: set CUSTOM_VISNING_UPDATE_TOKEN, GITHUB_TOKEN, or run gh auth login --hostname github.com")
	}
	client := updater.New(token)
	release, err := client.Latest(ctx)
	if err != nil {
		return err
	}
	if release.Tag == version {
		fmt.Fprintln(output, "Already running the latest release:", version)
		return nil
	}
	// Releases also use main-N.M-SHA and build-SHA versions, so semver ordering
	// would be misleading. Compare identity and show both versions explicitly.
	fmt.Fprintf(output, "Installed: %s; latest stable release: %s\n", version, release.Tag)
	if !download {
		return nil
	}
	asset, err := release.Archive(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not locate running binary: %w", err)
	}
	if executable, err = filepath.EvalSymlinks(executable); err != nil {
		return fmt.Errorf("could not resolve running binary: %w", err)
	}
	fmt.Fprintf(output, "Replace %s with the latest release? [y/N] ", executable)
	scanner := bufio.NewScanner(input)
	if !scanner.Scan() {
		if scanner.Err() != nil {
			return fmt.Errorf("could not read download confirmation")
		}
		fmt.Fprintln(output, "Download cancelled.")
		return nil
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	if answer != "y" && answer != "yes" && answer != "j" && answer != "ja" {
		fmt.Fprintln(output, "Download cancelled.")
		return nil
	}
	tmpDir, err := os.MkdirTemp("", "custom-visning-update-")
	if err != nil {
		return fmt.Errorf("could not create update workspace")
	}
	defer os.RemoveAll(tmpDir)
	path, err := client.Download(ctx, release, asset, tmpDir)
	if err != nil {
		return err
	}
	if err := updater.ReplaceArchive(path, executable); err != nil {
		return err
	}
	fmt.Fprintf(output, "Verified SHA-256 and replaced %s\n", executable)
	return nil
}
