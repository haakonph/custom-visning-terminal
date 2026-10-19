// Package app wires providers, persistence and the terminal program.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/cache"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/copilot"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/github"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/jira"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/mattermost"
	"github.com/haakonph/custom-visning-terminal/internal/local"
	"github.com/haakonph/custom-visning-terminal/internal/tui"
)

func Run(ctx context.Context, stderr io.Writer) error { return RunMode(ctx, stderr, false) }
func RunMode(ctx context.Context, stderr io.Writer, startJira bool) error {
	return RunProviders(ctx, stderr, startJira, false, "dev")
}
func RunProviders(ctx context.Context, stderr io.Writer, startJira, startCopilot bool, version string) error {
	if err := jira.Migrate(); err != nil {
		return fmt.Errorf("configuration migration: %w", err)
	}
	jiraConfig, configErr := jira.LoadConfig(jira.ConfigPath())
	if configErr != nil {
		return configErr
	}
	var jiraPattern *regexp.Regexp
	if value := os.Getenv("JIRA_PATTERN"); value != "" {
		var err error
		jiraPattern, err = regexp.Compile(value)
		if err != nil {
			return fmt.Errorf("invalid JIRA_PATTERN: %w", err)
		}
	}
	mmConfig, mmErr := mattermost.LoadConfig()
	if mmErr != nil {
		return mmErr
	}
	copilotOptions, copilotErr := copilot.OptionsFromEnv()
	if copilotErr != nil {
		return copilotErr
	}
	copilotClient, copilotErr := copilot.New(copilotOptions)
	if copilotErr != nil {
		return copilotErr
	}
	copilotClient.Start(ctx)
	defer func() {
		if err := copilotClient.Close(); err != nil {
			fmt.Fprintln(stderr, "Copilot store could not be saved")
		}
	}()
	if startCopilot {
		dashboard := tui.New(ctx, nil, nil).WithVersion(version).WithJiraPattern(jiraPattern).WithCopilot(copilotClient, true)
		return runDashboard(ctx, dashboard)
	}

	var jiraClient *jira.Client
	var jiraErr error
	baseURL := jiraConfig.BaseURL
	if value := os.Getenv("JIRA_BASE_URL"); value != "" {
		baseURL = value
	}
	if startJira || baseURL != "" || os.Getenv("JIRA_TOKEN") != "" {
		jiraClient, jiraErr = jira.New(baseURL, os.Getenv("JIRA_TOKEN"))
	}

	token, err := github.Token(ctx)
	if err != nil {
		if jiraClient != nil {
			runErr := runDashboard(ctx, tui.New(ctx, nil, nil).WithVersion(version).WithJiraPattern(jiraPattern).WithJira(jiraClient, true).WithCopilot(copilotClient, false))
			return runErr
		}
		if mmConfig.Server != "" {
			return runDashboard(ctx, tui.New(ctx, nil, nil).WithVersion(version).WithCopilot(copilotClient, false))
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
	dashboard := tui.New(ctx, client, persistence).WithVersion(version).WithJiraPattern(jiraPattern).WithCopilot(copilotClient, false)
	if endpoint := os.Getenv("MATTERMOST_WEBHOOK_URL"); endpoint != "" {
		sender, err := mattermost.New(endpoint)
		if err != nil {
			fmt.Fprintln(stderr, "Mattermost unavailable:", err)
		} else {
			dashboard = dashboard.WithMattermost(sender)
		}
	}
	configPath, dataDir, localErr := local.Directories()
	if localErr == nil {
		var config local.Config
		config, localErr = local.LoadConfig(configPath)
		if localErr == nil {
			var manager *local.WorktreeManager
			manager, localErr = local.New(dataDir, config)
			if localErr == nil {
				dashboard = dashboard.WithLocal(manager, config.Editors)
			}
		}
	}
	if localErr != nil {
		fmt.Fprintln(stderr, "Local worktrees unavailable:", localErr)
	}
	if jiraClient != nil {
		dashboard = dashboard.WithJira(jiraClient, startJira)
	}
	return runDashboard(ctx, dashboard)
}

func runDashboard(ctx context.Context, dashboard tui.Model) error {
	mmConfig, err := mattermost.LoadConfig()
	if err != nil {
		return err
	}
	if mmConfig.Server != "" {
		if mmConfig.Username == "" {
			return errors.New("MATTERMOST_USERNAME or mattermost.username in config.json is required")
		}
		api, err := mattermost.NewAPI(mmConfig.Server)
		if err != nil {
			return err
		}
		api.ExpectAccount(mmConfig.Username)
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			return err
		}
		path := filepath.Join(cacheDir, "custom-visning", "mattermost.sqlite")
		service, err := mattermost.OpenService(api, path, mmConfig.Server, mmConfig.Username)
		if err != nil {
			return fmt.Errorf("Mattermost cache: %w", err)
		}
		if err = mattermost.SaveConfig(mmConfig); err != nil {
			return fmt.Errorf("Mattermost configuration: %w", err)
		}
		dashboard = dashboard.WithMattermostReader(service, api, mattermost.Keychain{Server: mmConfig.Server, Account: mmConfig.Username}, mmConfig.Username, os.Getenv("MATTERMOST_TOKEN"))
	}

	_, err = tea.NewProgram(dashboard, tea.WithContext(ctx)).Run()
	return err
}
