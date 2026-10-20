package tui

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	appconfig "github.com/haakonph/custom-visning-terminal/internal/config"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

type notificationOptions struct {
	CI      bool `json:"ci"`
	Copilot bool `json:"copilot"`
}

func defaultNotificationOptions() notificationOptions { return notificationOptions{true, true} }

type notificationObservation struct {
	head     string
	ci       model.Status
	reviewID string
}

type notificationState struct {
	options notificationOptions
	seen    map[string]notificationObservation
}

type desktopNotification struct{ title, body, url string }
type notificationResult struct{ failed bool }

func (m *Model) loadNotifications() {
	options := defaultNotificationOptions()
	if err := appconfig.Section(appconfig.Path(), "notifications", &options); err != nil {
		m.notice = "Kunne ikke lese varslingsinnstillinger"
	}
	m.notifications.options = options
}

// Observe only successful, complete live lists. The first observation is a
// baseline, independent of disk cache, so startup never replays old events.
func (s *notificationState) observe(user string, prs []model.PullRequest) []desktopNotification {
	if s.seen == nil {
		s.seen = make(map[string]notificationObservation)
	}
	var events []desktopNotification
	for _, pr := range prs {
		if pr.URL == "" || user == "" || !strings.EqualFold(pr.Author, user) {
			continue
		}
		old, known := s.seen[pr.Key()]
		next := notificationObservation{head: pr.HeadOID, ci: pr.CI, reviewID: old.reviewID}
		if pr.CopilotReview != nil && pr.CopilotReview.ID != "" {
			next.reviewID = pr.CopilotReview.ID
		}
		label := fmt.Sprintf("%s #%d · %s", pr.Repository, pr.Number, pr.Title)
		if known && !pr.Draft {
			if s.options.CI && (pr.CI == model.Pass || pr.CI == model.Fail) &&
				(old.ci == model.Pending || (old.head != "" && pr.HeadOID != "" && old.head != pr.HeadOID)) {
				events = append(events, desktopNotification{"CI ferdig · " + string(pr.CI), label, pr.URL})
			}
			if s.options.Copilot && next.reviewID != "" && next.reviewID != old.reviewID {
				events = append(events, desktopNotification{"Copilot har levert review", label, pr.URL})
			}
		}
		// Unknown data must not erase a known pending CI or completed review.
		if (pr.CI == "" || pr.CI == model.Unknown) && old.head == next.head {
			next.ci = old.ci
		}
		s.seen[pr.Key()] = next
	}
	return events
}

func (m *Model) notifyPullRequests(msg refreshMsg) tea.Cmd {
	if msg.err != nil || !msg.result.Complete {
		return nil
	}
	events := m.notifications.observe(m.snapshot.User, msg.result.PRs)
	if len(events) == 0 {
		return nil
	}
	parent := m.ctx
	cmds := make([]tea.Cmd, 0, len(events))
	for _, event := range events {
		cmds = append(cmds, func() tea.Msg {
			// Waiting for a Linux click must not block later notifications or the UI.
			ctx, cancel := context.WithTimeout(parent, 10*time.Minute)
			defer cancel()
			err := sendDesktopNotification(ctx, runtime.GOOS, event)
			return notificationResult{failed: err != nil && ctx.Err() == nil}
		})
	}
	return tea.Batch(cmds...)
}

type notificationRunner func(context.Context, string, ...string) ([]byte, error)

func sendDesktopNotification(ctx context.Context, platform string, event desktopNotification) error {
	return deliverNotification(ctx, platform, event, func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, name, args...).Output()
	}, openBrowser)
}

func deliverNotification(ctx context.Context, platform string, event desktopNotification, run notificationRunner, open func(context.Context, string) error) error {
	parsed, err := url.Parse(event.url)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "github.com" || parsed.User != nil {
		return fmt.Errorf("invalid GitHub PR URL")
	}
	name, args := notificationCommand(platform, event)
	if name == "" {
		return fmt.Errorf("unsupported notification platform")
	}
	output, err := run(ctx, name, args...)
	if err != nil {
		return err
	}
	if platform == "linux" && strings.TrimSpace(string(output)) == "default" {
		return open(ctx, event.url)
	}
	return nil
}

func notificationCommand(platform string, event desktopNotification) (string, []string) {
	switch platform {
	case "darwin":
		// No shell or AppleScript interpolation. terminal-notifier handles the click.
		return "terminal-notifier", []string{"-title", event.title, "-message", event.body, "-open", event.url}
	case "linux":
		return "notify-send", []string{"--app-name=custom-visning", "--action=default=Åpne PR", "--wait", "--expire-time=30000", "--", event.title, event.body}
	default:
		return "", nil
	}
}
