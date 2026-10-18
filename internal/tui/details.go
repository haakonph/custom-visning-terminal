package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

type detailState struct {
	pr      model.PullRequest
	data    model.PRDetails
	loading bool
	err     error
	offset  int
	request uint64
}
type detailMsg struct {
	request uint64
	data    model.PRDetails
	err     error
}

func (m *Model) loadDetails() tea.Cmd {
	source, ok := m.source.(model.DetailSource)
	if !ok {
		m.detail.err = fmt.Errorf("Detaljer støttes ikke av denne datakilden")
		return nil
	}
	m.detailSequence++
	m.detail.request = m.detailSequence
	m.detail.loading = true
	m.detail.err = nil
	request, pr, parent := m.detail.request, m.detail.pr, m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
		defer cancel()
		data, err := source.Details(ctx, pr)
		return detailMsg{request: request, data: data, err: err}
	}
}
func (m Model) detailLines() []string {
	d := m.detail
	lines := []string{fmt.Sprintf("%s #%d · %s", d.pr.Repository, d.pr.Number, d.pr.Title), "Esc tilbake · j/k ↑/↓ bla · au oppdater · en nettleser", ""}
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
