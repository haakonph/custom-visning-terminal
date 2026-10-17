package tui

const shortcuts = "l related • 1–4 views • j/k ↑/↓ move • Enter open • r refresh • / filter • ? help • q quit"
const helpText = `c           Go to Copilot sessions (local history and telemetry)
g           Go to GitHub (navigation mode)
J (Shift+j) Go to Jira (navigation mode)
l           Navigate to related Jira issues / loaded GitHub PRs
1  My Open PRs (excluding drafts)
2  My Draft PRs
3  My Review Requests (direct requests only)
4  Team Review Requests (all accessible memberships)
j/k or ↑/↓  Select pull request
Enter       Open pull request in browser
r           Refresh current view
/           Filter repository, title, author, label or team
Enter       Keep filter and return to navigation
Esc         Clear filter / close help
?           Toggle help
q / Ctrl+C  Quit
UNKNOWN means GitHub has not supplied a status.
Partial refreshes retain cached rows and the last full sync time.`
