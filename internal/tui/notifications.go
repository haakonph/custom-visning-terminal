package tui

import (
	"context"
	"fmt"
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

type desktopNotification struct{ title, body string }
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
				events = append(events, desktopNotification{"CI ferdig · " + string(pr.CI), label})
			}
			if s.options.Copilot && next.reviewID != "" && next.reviewID != old.reviewID {
				events = append(events, desktopNotification{"Copilot har levert review", label})
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
	return func() tea.Msg {
		result := notificationResult{}
		for _, event := range events {
			ctx, cancel := context.WithTimeout(parent, 5*time.Second)
			err := sendDesktopNotification(ctx, runtime.GOOS, event)
			cancel()
			result.failed = result.failed || err != nil
		}
		return result
	}
}

func sendDesktopNotification(ctx context.Context, platform string, event desktopNotification) error {
	name, args := notificationCommand(platform, event)
	if name == "" {
		return fmt.Errorf("unsupported notification platform")
	}
	return exec.CommandContext(ctx, name, args...).Run()
}

func notificationCommand(platform string, event desktopNotification) (string, []string) {
	switch platform {
	case "darwin":
		// Fixed AppleScript; repository text is passed as data, never code.
		return "osascript", []string{"-e", "on run argv\ndisplay notification (item 2 of argv) with title (item 1 of argv)\nend run", "--", event.title, event.body}
	case "linux":
		return "notify-send", []string{"--app-name=custom-visning", "--", event.title, event.body}
	default:
		return "", nil
	}
}
