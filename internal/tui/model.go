package tui

import (
	"context"
	"regexp"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/jira"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/pullrequests"
	"github.com/haakonph/custom-visning-terminal/internal/relations"
	"github.com/haakonph/custom-visning-terminal/internal/syncstate"
	"github.com/haakonph/custom-visning-terminal/internal/usage"
)

type Cache = pullrequests.Cache
type Model struct {
	page           page
	jiraSession    syncstate.Scope
	copilotSession syncstate.Scope
	chatSession    syncstate.Scope
	webhookSession syncstate.Scope
	localIO        *localIOState
	views          *viewCache
	relations      *relations.Index

	folders          folderState
	usage            *usage.Recorder
	usageView        bool
	usageOffset      int
	usagePeriod      int
	usageNotice      string
	notifications    notificationState
	auto             refreshState
	setup            setupState
	version          string
	previousProvider string
	entities         *entityState
	dialog           *entityDialog
	extraChosen      map[string]model.TaskReference
	sequenceRefs     []model.TaskReference

	sequence       string
	sequenceNotice string
	jiraAction     *jiraActionState
	jiraChosen     map[string]jira.Issue

	detail               *detailState
	detailSequence       uint64
	mmReader             *mattermostReader
	settings             settingsState
	snoozes              *snoozeState
	mattermost           *mattermostState
	chosen               map[string]struct{}
	tasks                taskState
	copilot              *copilotState
	local                *localState
	jira                 *jiraState
	githubHiddenPrefixes []string
	jiraPattern          *regexp.Regexp
	relatedKey           string
	ctx                  context.Context
	cancel               context.CancelFunc
	prs                  *pullrequests.State
	active               model.View
	selected             int
	width, height        int
	input                textinput.Model
	filtering, help      bool
	helpOffset           int
	booting              bool
	loading              [model.ViewCount]bool
	warnings             [model.ViewCount]string
	notice               string
	saving, dirty        bool
	open                 func(context.Context, string) error
}
type bootMsg struct {
	token    syncstate.Token
	snapshot model.Snapshot
	err      error
	warning  string
}
type refreshMsg struct {
	token     syncstate.Token
	freshRows []model.PullRequest

	prepared bool
	baseRows []model.PullRequest

	autoMergeVersion uint64
	view             model.View
	result           model.Result
	err              error
	at               time.Time
}
type cacheMsg struct {
	err        error
	generation uint64
}
type openMsg struct{ err error }

func New(ctx context.Context, source model.Source, cache Cache) Model {
	ctx, cancel := context.WithCancel(ctx)
	input := textinput.New()
	input.Placeholder = "repository, title, author, label or team"
	input.Prompt = "/ "
	input.CharLimit = 256
	s := newSnooze()
	tasks := newTasks()
	tasks.snoozes = s
	io := &localIOState{}
	tasks.io = io
	m := Model{prs: pullrequests.New(ctx, source, cache), relations: &relations.Index{}, localIO: io, views: &viewCache{}, auto: refreshState{jira: map[string]*refreshJob{}}, entities: newEntities(), extraChosen: map[string]model.TaskReference{}, chosen: map[string]struct{}{}, snoozes: s, tasks: tasks, ctx: ctx, cancel: cancel, input: input, width: 100, height: 24, booting: true, open: openBrowser}
	m.loadRefresh()
	m.loadNotifications()
	m.loadJiraPattern()
	m.loadGitHubDisplay()
	return m
}
func (m Model) initProviders() tea.Cmd {
	cmds := []tea.Cmd{}
	if m.copilot != nil && m.copilot.client != nil {
		cmds = append(cmds, m.refreshCopilot())
	}
	if m.page == pageJira {
		cmds = append(cmds, m.refreshJira())
	}
	if m.prs.Source() != nil {
		cmds = append(cmds, m.bootstrap())
	}
	return tea.Batch(cmds...)
}
func (m Model) bootstrap() tea.Cmd {
	token := m.prs.Begin("bootstrap")
	if m.prs.Source() == nil {
		return nil
	}
	source, cache, parent := m.prs.Source(), m.prs.Cache(), m.prs.Context()
	return func() tea.Msg {
		// A credential-scoped cache supplies the previously verified identity,
		// allowing immediate startup even when the network is unavailable.
		if cache != nil {
			cached, err := cache.Load("")
			if err == nil && cached.User != "" {
				return bootMsg{token: token, snapshot: cached}
			}
		}
		ctx, cancel := context.WithTimeout(parent, 30*time.Second)
		defer cancel()
		user, err := source.User(ctx)
		if err != nil {
			return bootMsg{token: token, err: err}
		}
		snap := model.Snapshot{Version: 1, User: user}
		warning := ""
		if cache != nil {
			cached, err := cache.Load(user)
			if err == nil {
				snap = cached
			} else {
				warning = "Cache could not be read; refreshing from GitHub"
			}
		}
		return bootMsg{token: token, snapshot: snap, warning: warning}
	}
}
func (m *Model) refresh(view model.View) tea.Cmd {
	if view == model.SharedReviews {
		return tea.Batch(m.refresh(model.Reviews), m.refresh(model.TeamReviews))
	}
	if m.loading[view] || m.prs.Snapshot().User == "" {
		return nil
	}
	m.loading[view] = true
	m.auto.github[view].Started = time.Now()
	token := m.prs.Begin(pullrequests.ListResource(view))
	source, user, parent := m.prs.Source(), m.prs.Snapshot().User, m.prs.Context()
	version := m.prs.Revision()
	previous := m.prs.Snapshot().Datasets[view].PRs
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
		defer cancel()
		result, err := source.List(ctx, view, user)
		fresh := result.PRs
		result.PRs = append([]model.PullRequest(nil), result.PRs...)
		model.Sort(result.PRs)
		if err != nil || !result.Complete {
			result.PRs = model.Merge(previous, result.PRs)
		}
		return refreshMsg{token: token, freshRows: fresh, prepared: true, baseRows: previous, view: view, result: result, err: err, at: time.Now(), autoMergeVersion: version}
	}
}

// There is at most one cache write in flight. Later results are coalesced, so
// slow disk writes can never replace a newer snapshot with an older one.
func (m *Model) persist() tea.Cmd {
	if m.prs.Cache() == nil {
		return nil
	}
	if m.saving {
		m.dirty = true
		return nil
	}
	m.saving = true
	m.dirty = false
	snap, store := m.prs.Export(), m.prs.Cache()
	generation := m.prs.Generation()
	return func() tea.Msg { return cacheMsg{err: store.Save(snap), generation: generation} }
}

// WithVersion sets the build version displayed in help.
func (m Model) WithVersion(version string) Model {
	m.version = version
	return m
}

// WithJiraPattern selects the issue-key pattern used for PR links and navigation.
func (m Model) WithJiraPattern(pattern *regexp.Regexp) Model {
	if pattern != nil {
		m.jiraPattern = pattern
	}
	return m
}

func sameRows(a, b []model.PullRequest) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}

// Configure the client and its credential-scoped cache as one session.
func (m *Model) configureGitHub(source model.Source, cache Cache) {
	m.prs.Configure(source, cache)
	m.booting = source != nil
	m.loading = [model.ViewCount]bool{}
	m.warnings = [model.ViewCount]string{}
	m.dirty = m.saving
	m.auto.github = [model.ViewCount]refreshJob{}
	m.auto.bootstrap = refreshJob{}
	m.auto.detail = refreshJob{}
	clear(m.chosen)
	m.detail = nil
	m.invalidateViews(struct{}{})
}
