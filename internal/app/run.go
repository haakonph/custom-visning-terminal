// Package app wires providers, persistence and the terminal program.
package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/cache"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/copilot"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/github"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/jira"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/mattermost"
	"github.com/haakonph/custom-visning-terminal/internal/local"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/tui"
	"github.com/haakonph/custom-visning-terminal/internal/usage"
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
	copilotClient, copilotErr := copilot.NewDeferred(copilotOptions)
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
		return runDashboard(ctx, dashboard, stderr)
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

	token, tokenErr := github.Token(ctx)
	var source model.Source
	var persistence tui.Cache
	if tokenErr == nil {
		client, err := github.New(token)
		if err != nil {
			return err
		}
		source = client
		store, err := cache.Default(token)
		if err != nil {
			fmt.Fprintln(stderr, "Cache directory unavailable; running without disk cache")
		} else {
			persistence = store
		}
	}
	if jiraErr != nil {
		fmt.Fprintln(stderr, "Jira unavailable:", jiraErr)
	}
	dashboard := tui.New(ctx, source, persistence).WithVersion(version).WithJiraPattern(jiraPattern).WithCopilot(copilotClient, false)
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
		dashboard = dashboard.WithJira(jiraClient, startJira || source == nil)
	}
	if startJira && jiraClient == nil {
		dashboard = dashboard.WithSetup("jira")
	} else if source == nil && jiraClient == nil && (mmConfig.Server == "" || mmConfig.Username == "") {
		kind := "github"
		if mmConfig.Server != "" || mmConfig.Username != "" {
			kind = "mattermost"
		}
		dashboard = dashboard.WithSetup(kind)
	}
	return runDashboard(ctx, dashboard, stderr)
}

func runDashboard(ctx context.Context, dashboard tui.Model, stderr io.Writer) error {
	if os.Getenv("CUSTOM_VISNING_METRICS_DISABLED") == "1" {
		dashboard = dashboard.WithUsage(nil, "Deaktivert med CUSTOM_VISNING_METRICS_DISABLED=1.")
	} else if recorder, err := usage.Open(usage.DefaultPath()); err != nil {
		fmt.Fprintln(stderr, "Lokal bruksstatistikk er utilgjengelig; fortsetter uten innsamling.")
		dashboard = dashboard.WithUsage(nil, "Kunne ikke åpne den lokale statistikkdatabasen.")
	} else {
		dashboard = dashboard.WithUsage(recorder, "")
		recorder.Record("session", "start", "app", "")
		defer func() {
			if err := recorder.Close(); err != nil {
				fmt.Fprintln(stderr, "Kunne ikke lagre lokal bruksstatistikk.")
			}
		}()
	}
	configured, err := dashboard.WithMattermostConfig()
	if err != nil {
		dashboard = dashboard.WithSetupProblem("mattermost", "Kunne ikke klargjøre Mattermost: "+err.Error())
	} else {
		dashboard = configured
	}

	dashboard = dashboard.WithStartupSetup()
	_, err = tea.NewProgram(dashboard, tea.WithContext(ctx)).Run()
	return err
}
