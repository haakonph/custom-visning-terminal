package tui

import (
	"context"
	"regexp"
	"runtime"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/jira"
	"github.com/haakonph/custom-visning-terminal/internal/inputsource"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

type Cache interface {
	Load(string) (model.Snapshot, error)
	Save(model.Snapshot) error
}
type Model struct {
	keyboard         keyboardState
	notifications    notificationState
	auto             refreshState
	setup            setupState
	version          string
	previousProvider string
	entities         *entityState
	dialog           *entityDialog
	extraChosen      map[string]model.TaskReference
	sequenceRefs     []model.TaskReference
	internalKey      bool

	sequence          string
	sequenceNotice    string
	autoMergeBusy     bool
	reviewRequestBusy bool
	autoMergeVersion  uint64
	jiraAction        *jiraActionState
	jiraChosen        map[string]jira.Issue

	detail               *detailState
	detailSequence       uint64
	mmReader             *mattermostReader
	settings             settingsState
	snoozes              *snoozeState
	mattermost           *mattermostState
	chosen               map[string]model.PullRequest
	tasks                taskState
	copilot              *copilotState
	copilotActive        bool
	local                *localState
	jira                 *jiraState
	jiraActive           bool
	githubHiddenPrefixes []string
	jiraPattern          *regexp.Regexp
	relatedKey           string
	ctx                  context.Context
	cancel               context.CancelFunc
	source               model.Source
	cache                Cache
	snapshot             model.Snapshot
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
	snapshot model.Snapshot
	err      error
	warning  string
}
type refreshMsg struct {
	autoMergeVersion uint64
	view             model.View
	result           model.Result
	err              error
	at               time.Time
}
type cacheMsg struct{ err error }
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
	m := Model{auto: refreshState{jira: map[string]*refreshJob{}}, entities: newEntities(), extraChosen: map[string]model.TaskReference{}, chosen: map[string]model.PullRequest{}, snoozes: s, tasks: tasks, ctx: ctx, cancel: cancel, source: source, cache: cache, input: input, width: 100, height: 24, booting: true, open: openBrowser}
	m.loadRefresh()
	m.loadNotifications()
	m.loadJiraPattern()
	m.loadGitHubDisplay()
	m.loadKeyboard()
	if runtime.GOOS == "darwin" {
		m.keyboard.currentSource = inputsource.Current
	}
	return m
}
func (m Model) initProviders() tea.Cmd {
	cmds := []tea.Cmd{}
	if m.copilotActive {
		cmds = append(cmds, m.refreshCopilot())
	}
	if m.jiraActive {
		cmds = append(cmds, m.refreshJira())
	}
	if m.source != nil {
		cmds = append(cmds, m.bootstrap())
	}
	return tea.Batch(cmds...)
}
func (m Model) bootstrap() tea.Cmd {
	if m.source == nil {
		return nil
	}
	return func() tea.Msg {
		// A credential-scoped cache supplies the previously verified identity,
		// allowing immediate startup even when the network is unavailable.
		if m.cache != nil {
			cached, err := m.cache.Load("")
			if err == nil && cached.User != "" {
				return bootMsg{snapshot: cached}
			}
		}
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		user, err := m.source.User(ctx)
		if err != nil {
			return bootMsg{err: err}
		}
		snap := model.Snapshot{Version: 1, User: user}
		warning := ""
		if m.cache != nil {
			cached, err := m.cache.Load(user)
			if err == nil {
				snap = cached
			} else {
				warning = "Cache could not be read; refreshing from GitHub"
			}
		}
		return bootMsg{snapshot: snap, warning: warning}
	}
}
func (m *Model) refresh(view model.View) tea.Cmd {
	if m.loading[view] || m.snapshot.User == "" {
		return nil
	}
	m.loading[view] = true
	m.auto.github[view].started = time.Now()
	source, user, parent := m.source, m.snapshot.User, m.ctx
	version := m.autoMergeVersion
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
		defer cancel()
		result, err := source.List(ctx, view, user)
		return refreshMsg{view: view, result: result, err: err, at: time.Now(), autoMergeVersion: version}
	}
}

// There is at most one cache write in flight. Later results are coalesced, so
// slow disk writes can never replace a newer snapshot with an older one.
func (m *Model) persist() tea.Cmd {
	if m.cache == nil {
		return nil
	}
	if m.saving {
		m.dirty = true
		return nil
	}
	m.saving = true
	m.dirty = false
	snap, store := m.snapshot, m.cache
	return func() tea.Msg { return cacheMsg{err: store.Save(snap)} }
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
