package tui

import (
	"errors"
	"reflect"
	"testing"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestNotificationTransitions(t *testing.T) {
	s := notificationState{options: defaultNotificationOptions()}
	p := model.PullRequest{URL: "https://github.com/a/b/pull/1", Repository: "a/b", Number: 1, Author: "alice", HeadOID: "head1", CI: model.Pending}
	observe := func(want int) {
		t.Helper()
		if events := s.observe("alice", []model.PullRequest{p}); len(events) != want {
			t.Fatalf("got %+v, want %d events", events, want)
		}
	}
	observe(0) // First live observation, regardless of cache.
	p.CI = model.Unknown
	observe(0)
	p.CI = model.Pass
	p.CopilotReview = &model.CopilotReview{ID: "review1", Recommendation: "Approval"}
	observe(2)
	observe(0) // Another tab or refresh must not repeat notifications.
	p.CopilotReview = nil
	observe(0)
	p.CopilotReview = &model.CopilotReview{ID: "review1", Recommendation: "Changes"}
	observe(0) // Summary edits are not a new review.
	p.CopilotReview.ID = "review2"
	observe(1)
	p.HeadOID = "head2"
	p.CI = model.Fail
	observe(1) // A push can finish between polls.
	p.CI = model.Pending
	observe(0)
	p.CI = model.Fail
	observe(1) // A rerun on the same commit.
}

func TestNotificationScopeAndPreferences(t *testing.T) {
	for _, tc := range []struct {
		name    string
		author  string
		draft   bool
		enabled bool
		want    int
	}{
		{"own", "Alice", false, true, 2},
		{"other author", "bob", false, true, 0},
		{"draft", "alice", true, true, 0},
		{"disabled", "alice", false, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := notificationState{options: notificationOptions{tc.enabled, tc.enabled}}
			p := model.PullRequest{URL: "pr", Author: tc.author, Draft: tc.draft, CI: model.Pending}
			s.observe("alice", []model.PullRequest{p})
			p.CI = model.Pass
			p.CopilotReview = &model.CopilotReview{ID: "new"}
			if got := s.observe("alice", []model.PullRequest{p}); len(got) != tc.want {
				t.Fatal(got)
			}
			s.options = defaultNotificationOptions()
			if got := s.observe("alice", []model.PullRequest{p}); len(got) != 0 {
				t.Fatal("replayed events", got)
			}
		})
	}
	s := notificationState{options: defaultNotificationOptions()}
	p := model.PullRequest{URL: "pr", Author: "alice", CI: model.Pass, CopilotReview: &model.CopilotReview{ID: "old"}}
	if got := s.observe("alice", []model.PullRequest{p}); len(got) != 0 {
		t.Fatal("startup noise", got)
	}
}

func TestNotificationsIgnoreIncompleteResults(t *testing.T) {
	m := ready(t)
	p := model.PullRequest{URL: "pr", Author: "alice", CI: model.Pending}
	m.notifications.observe("alice", []model.PullRequest{p})
	p.CI = model.Pass
	for _, msg := range []refreshMsg{
		{result: model.Result{PRs: []model.PullRequest{p}}},
		{result: model.Result{PRs: []model.PullRequest{p}, Complete: true}, err: errors.New("offline")},
	} {
		if m.notifyPullRequests(msg) != nil {
			t.Fatal("notified on incomplete response")
		}
	}
	if m.notifyPullRequests(refreshMsg{result: model.Result{PRs: []model.PullRequest{p}, Complete: true}}) == nil {
		t.Fatal("lost pending transition")
	}
}

func TestNotificationTextIsPassedAsData(t *testing.T) {
	event := desktopNotification{`CI "done"`, "$(touch /tmp/unsafe)\n<script>"}
	name, args := notificationCommand("darwin", event)
	if name != "osascript" || !reflect.DeepEqual(args[len(args)-3:], []string{"--", event.title, event.body}) {
		t.Fatal(name, args)
	}
	name, args = notificationCommand("linux", event)
	if name != "notify-send" || !reflect.DeepEqual(args, []string{"--app-name=custom-visning", "--", event.title, event.body}) {
		t.Fatal(name, args)
	}
}

func TestNotificationSettingsApplyWithoutRestart(t *testing.T) {
	m := jiraModel(t)
	defer m.cancel()
	m, _ = sequence(m, "vs")
	for i, field := range settingFields {
		if field.path == "notifications.ci" {
			m = editSetting(t, m, i, "false")
			if m.notifications.options.CI || !m.notifications.options.Copilot {
				t.Fatal("CI setting did not apply independently", m.notifications.options)
			}
		}
	}
	m.loadNotifications()
	if m.notifications.options.CI || !m.notifications.options.Copilot {
		t.Fatal("notification preferences were not persisted", m.notifications.options)
	}
}
