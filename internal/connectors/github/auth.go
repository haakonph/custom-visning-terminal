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
	return tokenWithEnv(ctx, os.Getenv)
}

// UpdateToken prefers the dedicated release credential. Fallback is used only
// when it is absent, never after an authentication or permission failure.
func UpdateToken(ctx context.Context) (string, error) {
	if token := strings.TrimSpace(os.Getenv("CUSTOM_VISNING_UPDATE_TOKEN")); token != "" {
		return token, nil
	}
	return tokenWithEnv(ctx, os.Getenv)
}

func tokenWithEnv(ctx context.Context, getenv func(string) string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return ResolveToken(ctx, getenv, func(ctx context.Context) ([]byte, error) {
		return exec.CommandContext(ctx, "gh", "auth", "token", "--hostname", "github.com").Output()
	})
}
