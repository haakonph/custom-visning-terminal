# TUI-arkitektur

## Ansvarsgrenser

`internal/tui/shell.go` er den felles kanten mot Bubble Tea. Shell-et eier
livssyklus, aktiv skjerm, global navigasjon, vindusstørrelse og routing av
meldinger. Det skal ikke kjenne detaljene i Jira-, Copilot-, oppgave- eller
GitHub-funksjonene.

Skjermene eier hver sin tilstand, kommandoer, tastaturhåndtering og rendering.
Bakgrunnsresultater rutes etter meldingstype og skjermens generasjon også når
skjermen ikke er aktiv. Slik kan en bruker bytte skjerm uten å forkaste en
pågående henting eller overskrive en nyere lokal endring.

Planlagt plassering av skjermkode:

- `internal/tui/github/` — PR-datasett, filter, cache og worktree-adapter.
- `internal/tui/jira/` — JQL, lagrede søk, filtre og Jira-cache.
- `internal/tui/copilot/` — sesjoner, diagnostikk og IntelliJ-åpning.
- `internal/tui/tasks/` — lokal oppgavelagring og referanser.
- `internal/tui/components/` — små, stateless input- og layout-hjelpere.

Connectors og local er bevisst utenfor TUI-skjermene. De eksponerer små
provider-/operasjonsgrensesnitt og skal ikke importere TUI.

## Meldingsflyt

En skjerm kan starte en `tea.Cmd` og returnere et typespesifikt resultat. Shell
sender resultatet til eieren uavhengig av hvilken skjerm som er synlig. Globale
meldinger (`WindowSizeMsg`, avslutning og navigasjon) behandles av shell-et.
Generasjonsfelt på Jira- og worktree-resultater, samt én-pågående-refresh per
GitHub-view, gjør sene resultater ufarlige.

## Endringsregel

En funksjons-PR skal normalt bare endre sin skjermmappe og eventuelle
skjermspesifikke tester/dokumentasjon. Shell-endringer er for nye globale
interaksjoner eller routingbehov og skal ledsages av en routing-regresjonstest.
