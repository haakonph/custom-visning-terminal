// Package app wires providers, persistence and the terminal program.
package app

import (
	"context"
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/cache"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/github"
	"github.com/haakonph/custom-visning-terminal/internal/tui"
)

func Run(ctx context.Context, stderr io.Writer) error {
	token, err := github.Token(ctx)
	if err != nil {
		return err
	}
	client, err := github.New(token)
	if err != nil {
		return err
	}
	store, err := cache.Default(token)
	var persistence tui.Cache
	if err != nil {
		fmt.Fprintln(stderr, "Cache directory unavailable; running without disk cache")
	} else {
		persistence = store
	}
	_, err = tea.NewProgram(tui.New(ctx, client, persistence), tea.WithContext(ctx)).Run()
	return err
}
