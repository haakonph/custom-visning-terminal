package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/haakonph/custom-visning-terminal/internal/connectors/github"
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
	fmt.Fprintf(output, "Download %s to the current directory? [y/N] ", asset.Name)
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
	path, err := client.Download(ctx, release, asset, ".")
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "Verified SHA-256; saved %s\nExtract the archive and follow the installation instructions in README.md.\nOn macOS, if quarantined: xattr -d com.apple.quarantine custom-visning\n", path)
	return nil
}
