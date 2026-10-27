package tui

// Exactly one primary page is active. Dialogs and overlays keep their own state
// and do not erase each page's selection, filter or editor draft.
type page uint8

const (
	pageGitHub page = iota
	pageTasks
	pageJira
	pageCopilot
	pageChat
)

func (p page) provider() string {
	switch p {
	case pageTasks:
		return "task"
	case pageJira:
		return "jira"
	case pageCopilot:
		return "copilot"
	case pageChat:
		return "mattermost"
	default:
		return "github"
	}
}
func (m *Model) setPageFlag(target page, active bool) {
	if active {
		m.page = target
	} else if m.page == target {
		m.page = pageGitHub
	}
}
