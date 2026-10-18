package tui

import (
	"context"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

// screen identifies a feature-owned surface. Screens receive messages even
// while inactive; this is important for keeping background refreshes and
// telemetry up to date without making the shell own feature state.
type screen interface {
	ID() string
	Update(tea.Msg) (tea.Cmd, bool)
	View() string
}

// shellState is deliberately limited to lifecycle and presentation concerns.
// Feature state belongs in the feature state types (tasksState, jiraState,
// copilotState and the GitHub dataset) below Model.
type shellState struct {
	ctx       context.Context
	cancel    context.CancelFunc
	active    model.View
	width     int
	height    int
	input     textinput.Model
	filtering bool
	help      bool
	booting   bool
	notice    string
	open      func(context.Context, string) error
}

// shell is the boundary used by the feature screens. Keeping this contract
// in the shell package lets future screens live in their own packages without
// importing the application model or one another.
type shell interface {
	Context() context.Context
	Size() (int, int)
	Navigate(model.View)
	Notice(string)
}

func (m Model) Context() context.Context { return m.ctx }
func (m Model) Size() (int, int)         { return m.width, m.height }
func (m *Model) Navigate(view model.View) {
	m.active = view
	m.selected = 0
}
func (m *Model) Notice(message string) { m.notice = message }
