// Package app wires providers, persistence and the terminal program.
package app

import (
	"context"
	"errors"
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
func RunMode(ctx context.Context, stderr io.Writer, startJira bool) error {
	var jiraClient *jira.Client
	var jiraErr error
	if startJira || os.Getenv("JIRA_BASE_URL") != "" || os.Getenv("JIRA_TOKEN") != "" {
		jiraClient, jiraErr = jira.New(os.Getenv("JIRA_BASE_URL"), os.Getenv("JIRA_TOKEN"))
	}

	token, err := github.Token(ctx)
	if err != nil {
		if jiraClient != nil {
			_, runErr := tea.NewProgram(tui.New(ctx, nil, nil).WithJira(jiraClient, true), tea.WithContext(ctx)).Run()
			return runErr
		}
		return errors.Join(err, jiraErr)
	}
	if jiraErr != nil {
		fmt.Fprintln(stderr, "Jira unavailable:", jiraErr)
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
		dashboard = dashboard.WithJira(jiraClient, startJira)
	}
	_, err = tea.NewProgram(dashboard, tea.WithContext(ctx)).Run()
	return err
}
