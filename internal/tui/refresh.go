package tui

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	appconfig "github.com/haakonph/custom-visning-terminal/internal/config"
	"github.com/haakonph/custom-visning-terminal/internal/jira"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/pullrequests"
	"github.com/haakonph/custom-visning-terminal/internal/syncstate"
)

type refreshOptions struct {
	Enabled         bool   `json:"enabled"`
	Active          string `json:"active"`
	Background      string `json:"background"`
	Details         string `json:"details"`
	LocalActive     string `json:"local_active"`
	LocalBackground string `json:"local_background"`
}

func defaultRefreshOptions() refreshOptions {
	return refreshOptions{true, "1m", "5m", "30s", "15s", "1m"}
}
func refreshDuration(value string, fallback time.Duration) time.Duration {
	d, err := time.ParseDuration(value)
	if err != nil || d < 5*time.Second || d > 24*time.Hour {
		return fallback
	}
	return d
}

type refreshJob = syncstate.Job

type refreshState struct {
	jiraCancel context.CancelFunc
	options    refreshOptions
	github     [model.ViewCount]refreshJob
	bootstrap  refreshJob
	detail     refreshJob
	jira       map[string]*refreshJob
	localAt    time.Time
}
type refreshTick time.Time
type jiraBackgroundResult struct {
	token   syncstate.Token
	started time.Time
	query   string
	rows    []jira.Issue
	err     error
	at      time.Time
}

func refreshTimer() tea.Cmd {
	return tea.Tick(time.Second, func(at time.Time) tea.Msg { return refreshTick(at) })
}
func (m *Model) loadRefresh() {
	options := defaultRefreshOptions()
	if err := appconfig.Section(appconfig.Path(), "refresh", &options); err != nil {
		m.notice = "Kunne ikke lese oppdateringsinnstillinger"
	}
	m.auto.options = options
}

// All scheduling and completion state lives on the UI loop; commands only fetch.
func (m *Model) autoMessage(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch r := msg.(type) {
	case bootMsg:
		if r.token.Sequence != 0 && !m.prs.Accept("bootstrap", r.token) {
			return *m, nil, true
		}
		m.auto.bootstrap.Complete(time.Now(), r.err != nil)
	case refreshMsg:
		if r.token.Sequence != 0 && !m.prs.Match(pullrequests.ListResource(r.view), r.token) {
			return *m, nil, true
		}
		if r.prepared && sameRows(r.baseRows, m.prs.Snapshot().Datasets[r.view].PRs) {
			m.auto.github[r.view].Complete(time.Now(), r.err != nil || !r.result.Complete)
		}
	case detailMsg:
		if m.detail != nil && m.detail.request == r.request {
			m.auto.detail.Complete(time.Now(), r.err != nil || len(r.data.Warnings) > 0)
		}
	case jiraResult:
		if m.jira != nil && r.generation == m.jira.generation && (r.token.Sequence == 0 || m.jiraSession.Accept("query:"+r.query, r.token)) {
			if job := m.auto.jira[r.query]; job != nil {
				job.Complete(r.at, r.err != nil)
			}
		}
	case jiraBackgroundResult:
		if r.token.Sequence != 0 && !m.jiraSession.Accept("query:"+r.query, r.token) {
			return *m, nil, true
		}
		if job := m.auto.jira[r.query]; job == nil || !job.Started.Equal(r.started) {
			return *m, nil, true
		} else {
			job.Complete(r.at, r.err != nil)
		}
		if r.err == nil && m.jira != nil {
			// A newer foreground request always owns the visible result.
			if r.query != m.jira.query {
				client, data := m.jira.client, jira.Cached{Query: r.query, Issues: r.rows, SyncedAt: r.at}
				m.enqueueLocal(func() error { return client.SaveCache(data) }, func(m *Model, err error) {
					if err != nil {
						if job := m.auto.jira[r.query]; job != nil && job.Started.Equal(r.started) {
							job.Complete(r.at, true)
						}
						if m.jira != nil {
							m.jira.notice = "Kunne ikke lagre Jira-cache: " + err.Error()
						}
					}
				})
			}
		}
		return *m, nil, true
	case refreshTick:
		cmds := []tea.Cmd{refreshTimer()}
		o, now := m.auto.options, time.Time(r)
		if !o.Enabled || m.setup.wizard != nil || (m.localIO != nil && m.localIO.quitting) {
			return *m, tea.Batch(cmds...), true
		}
		active := refreshDuration(o.Active, time.Minute)
		background := refreshDuration(o.Background, 5*time.Minute)
		provider := m.visibleProvider()
		if m.prs.Source() != nil && m.prs.Snapshot().User == "" && !m.booting && m.auto.bootstrap.Due(now, active) {
			m.booting = true
			m.auto.bootstrap.Started = now
			cmds = append(cmds, m.bootstrap())
		}
		// One automatic GitHub list at a time, prioritizing the visible tab.
		busy := false
		for _, loading := range m.loading {
			busy = busy || loading
		}
		if m.prs.Source() != nil && m.prs.Snapshot().User != "" && !busy {
			visible := m.active
			if visible == model.SharedReviews {
				visible = model.Reviews
			}
			order := []model.View{visible}
			for v := model.Open; v < model.ViewCount; v++ {
				if v != visible {
					order = append(order, v)
				}
			}
			for _, v := range order {
				interval := background
				if provider == "github" && (v == visible || m.active == model.SharedReviews && v == model.TeamReviews) {
					interval = active
				}
				if m.auto.github[v].Due(now, interval) {
					cmds = append(cmds, m.refresh(v))
					break
				}
			}
		}
		if m.detail != nil && !m.detail.loading && m.auto.detail.Due(now, refreshDuration(o.Details, 30*time.Second)) {
			cmds = append(cmds, m.loadDetails())
		}
		if j := m.jira; j != nil && !j.loading && j.mode == "" && m.jiraAction == nil {
			// Track only queries the user has actually opened, never every saved query.
			if job := m.auto.jira[j.query]; job != nil {
				interval := background
				if provider == "jira" {
					interval = active
				}
				if job.Due(now, interval) {
					cmds = append(cmds, m.refreshJira())
				}
			}
			backgroundBusy := j.loading
			for _, job := range m.auto.jira {
				backgroundBusy = backgroundBusy || job.Busy
			}
			if !backgroundBusy {
				var oldest string
				for query, job := range m.auto.jira {
					if query != j.query && job.Due(now, background) && (oldest == "" || job.Finished.Before(m.auto.jira[oldest].Finished)) {
						oldest = query
					}
				}
				if oldest != "" {
					query, client, parent := oldest, j.client, m.jiraSession.Context(m.ctx)
					token := m.jiraSession.Begin("query:" + query)
					requestCtx, cancelRequest := context.WithCancel(parent)
					m.auto.jiraCancel = cancelRequest
					job := m.auto.jira[query]
					job.Busy = true
					job.Started = now
					cmds = append(cmds, func() tea.Msg {
						ctx, cancel := context.WithTimeout(requestCtx, 2*time.Minute)
						defer cancel()
						rows, err := client.Search(ctx, query)
						return jiraBackgroundResult{token: token, started: now, query: query, rows: rows, err: err, at: time.Now()}
					})
				}
			}
		}
		localInterval := refreshDuration(o.LocalBackground, time.Minute)
		if provider == "github" {
			localInterval = refreshDuration(o.LocalActive, 15*time.Second)
		}
		if m.local != nil && !m.local.statusLoading && now.Sub(m.auto.localAt) >= localInterval {
			m.auto.localAt = now
			cmds = append(cmds, m.refreshLocalWithNetwork(false))
		}
		return *m, tea.Batch(cmds...), true
	}
	return *m, nil, false
}
func syncAge(at time.Time) string {
	if at.IsZero() {
		return "aldri"
	}
	seconds := max(0, int(time.Since(at).Seconds()))
	if seconds < 60 {
		return fmt.Sprintf("%d sek siden", seconds)
	}
	return fmt.Sprintf("%d min siden", seconds/60)
}
