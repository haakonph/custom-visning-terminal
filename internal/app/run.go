// Package app wires providers, persistence and the terminal program.
package app

import (
	"context"
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/cache"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/github"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/jira"
	"github.com/haakonph/custom-visning-terminal/internal/tui"
)

func Run(ctx context.Context, stderr io.Writer) error { return RunMode(ctx, stderr, false) }
func RunMode(ctx context.Context, stderr io.Writer, jiraOnly bool) error {
	var jiraClient *jira.Client
	if jiraOnly || os.Getenv("JIRA_BASE_URL") != "" || os.Getenv("JIRA_TOKEN") != "" {
		var err error
		jiraClient, err = jira.New(os.Getenv("JIRA_BASE_URL"), os.Getenv("JIRA_TOKEN"))
		if err != nil {
			return err
		}
	}
	if jiraOnly {
		_, err := tea.NewProgram(tui.New(ctx, nil, nil).WithJira(jiraClient, true), tea.WithContext(ctx)).Run()
		return err
	}
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
	dashboard := tui.New(ctx, client, persistence)
	if jiraClient != nil {
		dashboard = dashboard.WithJira(jiraClient, false)
	}
	_, err = tea.NewProgram(dashboard, tea.WithContext(ctx)).Run()
	return err
}
