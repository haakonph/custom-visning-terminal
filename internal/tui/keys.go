package tui

const shortcuts = "T oppgaver • t ny fra valgt • a legg til • Space select • o editor • O choose • l related • 1–4 views • j/k ↑/↓ move • Enter open • r refresh • / filter • ? help • q quit"
const helpText = `T           Open tasks; Tab selects the task used by a
t           Create a task wrapping the selected Jira issue / PR
a           Add selected Jira issue / PR to current task
c           Go to Copilot sessions (local history and telemetry)
g           Go to GitHub (navigation mode)
J (Shift+j) Go to Jira (navigation mode)
l           Navigate to related Jira issues / loaded GitHub PRs
1  My Open PRs (excluding drafts)
2  My Draft PRs
3  My Review Requests (direct requests only)
4  Team Review Requests (all accessible memberships)
j/k or ↑/↓  Select pull request
Space       Toggle PR selection (persists across views and filters)
o / O       Open selected PRs in default editor / choose editor
            Without selection, use highlighted PR
Enter       Open pull request in browser
r           Refresh current view
/           Filter repository, title, author, label or team
Enter       Keep filter and return to navigation
Esc         Clear filter / close help
?           Toggle help
q / Ctrl+C  Quit
UNKNOWN means GitHub has not supplied a status.
Partial refreshes retain cached rows and the last full sync time.`
