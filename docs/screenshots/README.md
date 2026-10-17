# Jira-visningen

Skjermbildene viser den bygde terminalappen (`custom-visning --jira`) mot en lokal HTTPS-testserver med eksempeldata. Ingen ekte Jira-saker eller token er brukt. Terminalutdata er fanget fra en PTY og gjengitt i xterm.js; vindusrammen er lagt til for presentasjon.

- `jira-dashboard.png`: åpne saker med status, prioritet, ansvarlig og detaljer.
- `jira-filters.png`: redigering av kombinerbare hurtigfiltre.
- `jira-jql.png`: redigering av egen JQL-spørring.
- `jira-saved-query.png`: lagret søk «Mine feil» med tre treff.

Bildene illustrerer grensesnittet og dokumenterer ikke tilkobling til en faktisk Jira-installasjon.

Oppdatert med nummererte faner: `1–4` for standardvisninger og `5–9` for de første lagrede søkene. `g` åpner GitHub, og `J` (Shift+j) åpner Jira.


## Oppgavesiden

`tasks-overview.png` viser 15 eksempeloppgaver med oppgaveliste til venstre og
referansene til valgt oppgave til høyre. Innholdet er gjengitt fra appens faktiske
`Model.View()` med lokale eksempeldata, i et terminalrutenett på 112 × 24 tegn.
ANSI-farger er gjengitt i PNG, med en vindusramme lagt til for presentasjon.
Ingen eksterne tjenester eller ekte saker er brukt.
