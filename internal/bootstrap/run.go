// Package bootstrap wires providers, persistence and the terminal program.
package bootstrap

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/accesslog"
	"github.com/haakonph/custom-visning-terminal/internal/conversations"
	"github.com/haakonph/custom-visning-terminal/internal/copilot"
	"github.com/haakonph/custom-visning-terminal/internal/jira"
	"github.com/haakonph/custom-visning-terminal/internal/local"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/pullrequests/github"
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
	mmConfig, mmErr := conversations.LoadConfig()
	if mmErr != nil {
		return mmErr
	}
	copilotOptions, copilotErr := copilot.OptionsFromEnv()
	if copilotErr != nil {
		return copilotErr
	}
	requestLog, logErr := accesslog.Open(accesslog.DefaultPath())
	if logErr != nil {
		fmt.Fprintln(stderr, "Kunne ikke åpne HTTP-loggen; fortsetter uten logging:", logErr)
	} else {
		defer func() {
			if err := requestLog.Close(); err != nil {
				fmt.Fprintln(stderr, "Kunne ikke skrive HTTP-loggen:", err)
			}
		}()
	}
	copilotOptions.AccessLog = requestLog
	// Keep metrics alive from receiver startup through its shutdown, so final
	// requests are recorded before the counters are flushed and closed.
	recorder, usageNotice := openUsage(stderr)
	if recorder != nil {
		defer func() {
			if err := recorder.Close(); err != nil {
				fmt.Fprintln(stderr, "Kunne ikke lagre lokal bruksstatistikk.")
			}
		}()
	}
	copilotOptions.Usage = recorder
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
		return runDashboard(ctx, dashboard, stderr, recorder, usageNotice)
	}

	var jiraClient *jira.Client
	var jiraErr error
	baseURL := jiraConfig.BaseURL
	if value := os.Getenv("JIRA_BASE_URL"); value != "" {
		baseURL = value
	}
	if startJira || baseURL != "" || os.Getenv("JIRA_TOKEN") != "" {
		jiraClient, jiraErr = jira.Connect(ctx, baseURL, os.Getenv("JIRA_TOKEN"), false)
	}

	connection, tokenErr := github.Connect(ctx, "", false)
	var source model.Source
	var persistence tui.Cache
	if tokenErr == nil {
		source, persistence = connection.Client, connection.Cache
		if connection.CacheWarning != nil {
			fmt.Fprintln(stderr, "Cache directory unavailable; running without disk cache")
		}
	}
	if jiraErr != nil {
		fmt.Fprintln(stderr, "Jira unavailable:", jiraErr)
	}
	dashboard := tui.New(ctx, source, persistence).WithVersion(version).WithJiraPattern(jiraPattern).WithCopilot(copilotClient, false)
	if endpoint, err := conversations.LoadWebhook(ctx); err != nil {
		fmt.Fprintln(stderr, "Mattermost webhook unavailable:", err)
	} else if endpoint != "" {
		sender, err := conversations.New(endpoint)
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
	return runDashboard(ctx, dashboard, stderr, recorder, usageNotice)
}

func openUsage(stderr io.Writer) (*usage.Recorder, string) {
	if os.Getenv("CUSTOM_VISNING_METRICS_DISABLED") == "1" {
		return nil, "Deaktivert med CUSTOM_VISNING_METRICS_DISABLED=1."
	}
	recorder, err := usage.Open(usage.DefaultPath())
	if err != nil {
		fmt.Fprintln(stderr, "Lokal bruksstatistikk er utilgjengelig; fortsetter uten innsamling.")
		return nil, "Kunne ikke åpne den lokale statistikkdatabasen."
	}
	return recorder, ""
}

func runDashboard(ctx context.Context, dashboard tui.Model, stderr io.Writer, recorder *usage.Recorder, usageNotice string) error {
	dashboard = dashboard.WithUsage(recorder, usageNotice)
	recorder.Record("session", "start", "app", "")
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
