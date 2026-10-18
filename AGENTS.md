# Prosjektinstruksjoner

- Dette prosjektet er https://github.com/haakonph/custom-visning-terminal.
- Gjør alle prosjektendringer på en egen gren.
- Push alltid grenen og opprett en pull request (PR) etter prosjektendringer. Ikke stopp ved lokale endringer eller commits. Brukeren har forhåndsgodkjent push og opprettelse av PR.
- Ikke push endringer direkte til hovedgrenen.

## Hvor funksjonsendringer hører hjemme

TUI-en er delt i en liten shell og funksjonseide skjermer. Shell-kode for aktiv
skjerm, global navigasjon, vindusstørrelse og meldingsruting hører hjemme i
`internal/tui/shell.go` og de globale tastatur-/layoutkomponentene i
`internal/tui/components/`. Nye eller endrede funksjonsregler skal legges i
sin skjermmappe: GitHub i `internal/tui/github/`, Jira i
`internal/tui/jira/`, Copilot i `internal/tui/copilot/` og oppgaver i
`internal/tui/tasks/`. En skjerm skal eie sin tilstand, sine meldinger,
hurtigtaster og rendering.

Provider- og lagringsintegrasjoner skal fortsatt ligge i `internal/connectors/`.
Worktree- og editoroperasjoner skal fortsatt ligge i `internal/local/`; GitHub-
skjermen skal bare bruke det avgrensede worktree-grensesnittet. Detaljert
funksjonsdokumentasjon vedlikeholdes under `docs/`, mens `README.md` bare skal
ha en kort oversikt og lenker.
