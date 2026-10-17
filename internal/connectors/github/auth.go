package github

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

type TokenRunner func(context.Context) ([]byte, error)

// ResolveToken is called once, at startup. CLI stderr is deliberately discarded.
func ResolveToken(ctx context.Context, getenv func(string) string, run TokenRunner) (string, error) {
	if token := strings.TrimSpace(getenv("GITHUB_TOKEN")); token != "" {
		return token, nil
	}
	data, err := run(ctx)
	if err != nil || strings.TrimSpace(string(data)) == "" {
		return "", errors.New("GitHub authentication missing: set GITHUB_TOKEN or run gh auth login --hostname github.com")
	}
	return strings.TrimSpace(string(data)), nil
}
func Token(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return ResolveToken(ctx, os.Getenv, func(ctx context.Context) ([]byte, error) {
		return exec.CommandContext(ctx, "gh", "auth", "token", "--hostname", "github.com").Output()
	})
}
