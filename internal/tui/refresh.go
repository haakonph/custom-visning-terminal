package tui

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	appconfig "github.com/haakonph/custom-visning-terminal/internal/config"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/jira"
	"github.com/haakonph/custom-visning-terminal/internal/model"
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

type refreshJob struct {
	started, finished time.Time
	failures          int
	busy              bool
}

func (j *refreshJob) complete(at time.Time, failed bool) {
	j.finished, j.busy = at, false
	if failed {
		j.failures = min(j.failures+1, 6)
	} else {
		j.failures = 0
	}
}
func (j refreshJob) due(now time.Time, interval time.Duration) bool {
	if j.busy {
		return false
	}
	delay := interval
	if j.failures > 0 {
		delay = max(delay, min(30*time.Minute, time.Minute*time.Duration(1<<j.failures)))
	}
	return now.Sub(maxTime(j.started, j.finished)) >= delay
}
func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

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
		m.auto.bootstrap.complete(time.Now(), r.err != nil)
	case refreshMsg:
		if r.prepared && sameRows(r.baseRows, m.snapshot.Datasets[r.view].PRs) {
			m.auto.github[r.view].complete(time.Now(), r.err != nil || !r.result.Complete)
		}
	case detailMsg:
		if m.detail != nil && m.detail.request == r.request {
			m.auto.detail.complete(time.Now(), r.err != nil || len(r.data.Warnings) > 0)
		}
	case jiraResult:
		if m.jira != nil && r.generation == m.jira.generation {
			if job := m.auto.jira[r.query]; job != nil {
				job.complete(r.at, r.err != nil)
			}
		}
	case jiraBackgroundResult:
		if job := m.auto.jira[r.query]; job == nil || !job.started.Equal(r.started) {
			return *m, nil, true
		} else {
			job.complete(r.at, r.err != nil)
		}
		if r.err == nil && m.jira != nil {
			// A newer foreground request always owns the visible result.
			if r.query != m.jira.query {
				client, data := m.jira.client, jira.Cached{Query: r.query, Issues: r.rows, SyncedAt: r.at}
				m.enqueueLocal(func() error { return client.SaveCache(data) }, func(m *Model, err error) {
					if err != nil {
						if job := m.auto.jira[r.query]; job != nil && job.started.Equal(r.started) {
							job.complete(r.at, true)
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
		if m.source != nil && m.snapshot.User == "" && !m.booting && m.auto.bootstrap.due(now, active) {
			m.booting = true
			m.auto.bootstrap.started = now
			cmds = append(cmds, m.bootstrap())
		}
		// One automatic GitHub list at a time, prioritizing the visible tab.
		busy := false
		for _, loading := range m.loading {
			busy = busy || loading
		}
		if m.source != nil && m.snapshot.User != "" && !busy {
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
				if m.auto.github[v].due(now, interval) {
					cmds = append(cmds, m.refresh(v))
					break
				}
			}
		}
		if m.detail != nil && !m.detail.loading && m.auto.detail.due(now, refreshDuration(o.Details, 30*time.Second)) {
			cmds = append(cmds, m.loadDetails())
		}
		if j := m.jira; j != nil && !j.loading && j.mode == "" && m.jiraAction == nil {
			// Track only queries the user has actually opened, never every saved query.
			if job := m.auto.jira[j.query]; job != nil {
				interval := background
				if provider == "jira" {
					interval = active
				}
				if job.due(now, interval) {
					cmds = append(cmds, m.refreshJira())
				}
			}
			backgroundBusy := j.loading
			for _, job := range m.auto.jira {
				backgroundBusy = backgroundBusy || job.busy
			}
			if !backgroundBusy {
				var oldest string
				for query, job := range m.auto.jira {
					if query != j.query && job.due(now, background) && (oldest == "" || job.finished.Before(m.auto.jira[oldest].finished)) {
						oldest = query
					}
				}
				if oldest != "" {
					query, client, parent := oldest, j.client, m.ctx
					requestCtx, cancelRequest := context.WithCancel(parent)
					m.auto.jiraCancel = cancelRequest
					job := m.auto.jira[query]
					job.busy = true
					job.started = now
					cmds = append(cmds, func() tea.Msg {
						ctx, cancel := context.WithTimeout(requestCtx, 2*time.Minute)
						defer cancel()
						rows, err := client.Search(ctx, query)
						return jiraBackgroundResult{started: now, query: query, rows: rows, err: err, at: time.Now()}
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
