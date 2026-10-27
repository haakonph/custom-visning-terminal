package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/syncstate"
)

type detailState struct {
	lines       []string
	lineWidth   int
	lineLoading bool
	lineAge     string

	synced  time.Time
	id      string
	data    model.PRDetails
	loading bool
	err     error
	offset  int
	request uint64
}
type detailMsg struct {
	token            syncstate.Token
	autoMergeVersion uint64
	request          uint64
	data             model.PRDetails
	err              error
}

func (m *Model) loadDetails() tea.Cmd {
	if m.detail.loading {
		return nil
	}
	if m.detail.request == 0 {
		m.auto.detail = refreshJob{}
	}
	source, ok := m.prs.Source().(model.DetailSource)
	if !ok {
		m.detail.err = fmt.Errorf("Detaljer støttes ikke av denne datakilden")
		m.auto.detail.Complete(time.Now(), true)
		return nil
	}
	m.detailSequence++
	m.detail.request = m.detailSequence
	m.detail.loading = true
	m.auto.detail.Started = time.Now()
	m.detail.err = nil
	request, pr, parent := m.detail.request, m.detailPR(), m.prs.Context()
	autoMergeVersion := m.prs.Revision()
	token := m.prs.Begin("details")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
		defer cancel()
		data, err := source.Details(ctx, pr)
		return detailMsg{token: token, request: request, data: data, err: err, autoMergeVersion: autoMergeVersion}
	}
}
func (m Model) detailLines() []string {
	d := m.detail
	pr := m.detailPR()
	age := syncAge(d.synced)
	if d.lines != nil && d.lineWidth == m.width && d.lineAge == age && d.lineLoading == d.loading {
		return d.lines
	}
	lines := []string{fmt.Sprintf("%s #%d · %s", m.githubName(pr.Repository), pr.Number, pr.DisplayTitle(m.jiraPattern)), "Esc tilbake · j/k ↑/↓ bla · au oppdater · en nettleser", "Oppdatert: " + syncAge(d.synced), ""}
	auto := ""
	if label := autoMergeLabel(pr); label != "" {
		auto = "Automerge: " + label + " · "
	}
	lines = append(lines, "Jira: "+strings.Join(pr.JiraKeys(m.jiraPattern), ", "), auto+"Queue: "+queueLabel(pr), "a+ automerge på (merge when ready) · a- av")
	if pr.Mergeability == model.Conflicting {
		lines = append(lines, "CONFLICT: PR-en har mergekonflikter som må løses før den kan merges.")
	}
	lines = append(lines, reviewSummaryLabel(pr))
	if pr.OpenComments != nil {
		lines = append(lines, fmt.Sprintf("Åpne kommentarer: %d (uløste reviewtråder)", *pr.OpenComments))
	}
	if pr.ReviewSummary != nil && pr.ReviewSummary.Fallback {
		lines = append(lines, "Vurderingene gjelder denne PR-en. GitHubs regler avgjør om den kan merges.")
	}
	if d.loading {
		lines = append(lines, "Laster detaljer …")
	}
	if d.err != nil {
		lines = append(lines, d.err.Error())
	}
	lines = append(lines, d.data.Warnings...)
	lines = append(lines, "", "Endrede filer")
	if len(d.data.Files) == 0 && !d.loading {
		lines = append(lines, "Ingen filer lastet.")
	}
	directories := map[string]bool{}
	for _, f := range d.data.Files {
		parts := strings.Split(f.Path, "/")
		for i := 1; i < len(parts); i++ {
			path := strings.Join(parts[:i], "/")
			if !directories[path] {
				lines = append(lines, strings.Repeat("  ", i-1)+parts[i-1]+"/")
				directories[path] = true
			}
		}
		line := fmt.Sprintf("%s%s [%s] +%d −%d", strings.Repeat("  ", len(parts)-1), parts[len(parts)-1], f.Status, f.Additions, f.Deletions)
		if f.PreviousPath != "" {
			line += " ← " + f.PreviousPath
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", "Actions / sjekker")
	if len(d.data.Checks) == 0 && !d.loading {
		lines = append(lines, "Ingen sjekker lastet.")
	}
	for _, c := range d.data.Checks {
		line := c.Name + ": " + c.Status
		if c.Conclusion != "" {
			line += " · " + c.Conclusion
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", "Kommentarer og reviews")
	if len(d.data.Comments) == 0 && !d.loading {
		lines = append(lines, "Ingen kommentarer lastet.")
	}
	for _, c := range d.data.Comments {
		title := c.Author
		if c.Path != "" {
			title += fmt.Sprintf(" · %s:%d", c.Path, c.Line)
		}
		lines = append(lines, "", title)
		lines = append(lines, strings.Split(strings.ReplaceAll(c.Body, "\r\n", "\n"), "\n")...)
	}
	var wrapped []string
	for _, line := range lines {
		wrapped = append(wrapped, strings.Split(ansi.Hardwrap(clean(line), max(1, m.width-2), true), "\n")...)
	}
	d.lines, d.lineWidth, d.lineAge = wrapped, m.width, age
	d.lineLoading = d.loading
	return wrapped
}
func (m Model) detailView() tea.View {
	lines := m.detailLines()
	height := max(1, m.height)
	offset := min(m.detail.offset, max(0, len(lines)-height))
	view := tea.NewView(strings.Join(lines[offset:min(len(lines), offset+height)], "\n"))
	view.AltScreen = true
	return view
}

// Failed sections keep their prior data until a complete section is available.
func retainDetailSections(old, next model.PRDetails) model.PRDetails {
	for _, warning := range next.Warnings {
		switch {
		case strings.HasPrefix(warning, "Filer:"):
			if len(old.Files) > 0 {
				next.Files = old.Files
			}
		case strings.HasPrefix(warning, "Actions:"):
			if len(old.Checks) > 0 {
				next.Checks = old.Checks
			}
		case strings.HasPrefix(warning, "Kommentarer:"), strings.HasPrefix(warning, "Kodekommentarer:"), strings.HasPrefix(warning, "Reviews:"):
			if len(old.Comments) > 0 {
				next.Comments = old.Comments
			}
		}
	}
	return next
}

func (m Model) detailPR() model.PullRequest {
	if m.detail == nil {
		return model.PullRequest{}
	}
	p, _ := m.prs.Lookup(m.detail.id)
	return p
}
