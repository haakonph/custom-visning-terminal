# Tastaturnavigasjon

Handlinger gjelder den aktive visningen og valget der. Menyene og `?` viser bare
handlingene for denne visningen. En meny avsluttes med én handling; handlinger
kan ikke kjedes gjennom andre typer eller relasjoner.

Eksempel: Fra GitHub kan `rt` åpne en relatert Jira-sak. Flere treff gir en
valgliste der Enter åpner saken. Bruk deretter `af` i Jira for å velge en
statusovergang. Den tidligere kjeden `atfq` er fjernet.

## Navigasjon og valg

| Tast | Handling |
| --- | --- |
| `j` / `k` / `↓` / `↑` | Velg rad; rull i detaljer |
| `h` / `l` / `←` / `→` | Bytt fane; i oppgaver byttes oppgave |
| `d` | Vis detaljer; i chat åpnes tråden |
| `/` | Søk eller filtrer i aktiv liste |
| `?` | Vis hjelp for aktiv visning |
| `Space` | Marker eller fjern markering av aktiv rad |
| `Shift+↑` / `Shift+↓` | Utvid markeringen |
| `x` | Fjern markeringer; behold søk og filtre |
| `z` + valgfritt tall + `h` / `d` / `m` / `D` | Utsett valget; uten tall brukes 1 |
| `zz` + valgfritt tall + tidsenhet | Vis og utsett hele sammenhengen |
| `Esc` | Avbryt meny/dialog, lukk detaljer eller fjern markeringer |
| `Enter` | Bekreft tekst eller valg i dialoger og valglister |
| `Ctrl+C` | Avslutt og kanseller bakgrunnsarbeid |

Detaljer bruker elementet som vises, selv om den underliggende listen har andre
markeringer. Fanebytte, søk og markeringskommandoer endrer ikke listen bak
detaljene. Tekstfelt mottar inntasting som tekst og starter ingen menyer.

Flervalg beholdes ved bytte av fane/filter innenfor samme type. Markeringer i
andre visninger eller en annen oppgaves referanser tas ikke med som skjulte mål.

For utsetting betyr `h` timer og `d` kalenderdager med samme lokale klokkeslett.
`m` og `D` betyr antall kalenderdager fram til kl. 06. `zz` viser medlemmene før
lagring; Esc avbryter. Utsetting er en egen dialog, ikke en kjede av handlinger.

## Visningsmenyen (`v`)

| Tast | Visning |
| --- | --- |
| `vo` / `vg` / `vt` / `va` / `vc` | Oppgaver / GitHub / Jira / AI / chat |
| `vs` / `vr` | Innstillinger / forrige hovedvisning |
| `vz` / `vb` | Utsatte / lokalt arkiv |
| `vh` | Oppsett for aktiv side |
| `vf` / `vj` | Hurtigfiltre / egen JQL; bare i Jira-listen |
| `vi` | Diagnostikk; bare i AI-visningen |
| `vk` | Velg kanaler; bare i chat |

## Handlingsmenyen (`a`)

| Visning | Taster og handlinger |
| --- | --- |
| Vanlige visninger | `au` oppdater, `as` lag meldingsutkast fra valget, `ak` arkiver lokalt |
| GitHub | `a+` automerge på, `a-` automerge av, `ar` be om Copilot-review, `aw` slett lokale worktrees |
| Jira | `af` flytt sak, `am` tilordne meg, `ar` fjern min tilordning |
| Jira-liste | `aq` lagre søk, `ax` nullstill filtre og JQL |
| Oppgaver | `an` ny tom oppgave, `ae` endre tittel, `ad` slett valgte oppgaver |
| Oppgavedetaljer | `ad` fjern valgte referanser fra oppgaven |
| Chat | `al` passordinnlogging, `ap` PAT, `av` merk kanal lest, `aw` slutt å overvåke, `ab` tøm lokal konto |
| Chat | `ah` hent eldre meldinger, `a↓` / `a↑` rull en halv side |
| Utsatte | `az` hent tilbake valgte elementer |
| Arkiv | `ak` gjenopprett valgte elementer |
| AI-diagnostikk | `au` oppdater |

Opprett en oppgave med `vo`, deretter `an`. Referanser kan kobles til etterpå med
koblingsmenyen. Opprettelse fra andre visninger er fjernet.

`as` åpner et redigerbart Markdown-utkast. I utkastet betyr `ee` rediger igjen og
`as` send til konfigurert Mattermost-webhook. Åpning av utkastet sender ingenting.

Jira-overganger hentes fra arbeidsflyten til de valgte sakene. Etter `af` velges
en overgang med j/k og Enter, eller `q` QA, `u` under arbeid, `a` avklaring og
`f` ferdig. En snarvei må ha en entydig overgang som alle valgte saker støtter.
Tasten kan skrives mens overganger lastes. Esc avbryter før utføring. Ved feil
stoppes resten av flervalget; tidligere saker kan allerede være oppdatert.

## Åpningsmenyen (`e`)

| Visning | Taster og handlinger |
| --- | --- |
| GitHub | `ee` åpne valgt PR i IntelliJ, `en` åpne i nettleser |
| GitHub-liste | `ev` velg editor med j/k og Enter |
| Jira | `en` åpne valgt sak i nettleser |
| AI | `ee` åpne chatmappe, `ew` åpne worktree i IntelliJ |
| Chat | `en` åpne valgt melding i nettleser, `el` velg lenker i meldingen |
| Oppgavedetaljer | `en` åpne valgt referanse i nettleser |

AI bruker nå `ee` og `ew` i stedet for `eee` og `eew`. Hvis den foretrukne lokale
mappen mangler, brukes prosjektroten og deretter den andre mappen.
AI-sesjoner uten nettadresse kan åpnes i editor. Editorhandlinger følger ingen
relasjoner automatisk. Åpne først ønsket PR eller AI-sesjon i dens visning.

## Relasjoner og lokale koblinger

`r` åpner relaterte elementer, mens `t` velger mål for en lokal kobling:

| Type | Relatert | Koble til |
| --- | --- | --- |
| Oppgaver | `ro` | `to` |
| GitHub | `rg` | `tg` |
| Jira | `rt` | `tt` |
| AI | `ra` | `ta` |
| Chat | `rc` | `tc` |

Ett relatert treff åpnes direkte. Flere treff vises i en søkbar valgliste.
Valglister tilbyr navigasjon og bekreftelse. Åpne treffet med Enter før du utfører
handlinger. Ved kobling lagrer Enter koblingene mellom valgte kilder og mål.

Koblinger lagres lokalt i `links.sqlite` (`CUSTOM_VISNING_LINKS` kan overstyre
plasseringen). Oppgaver beholder også egne referanser. Arkivering er lokal;
sletting gjelder bare interne oppgaver og deres referanser. Eksisterende lagrede
koblinger beholdes. Jira-nøkler, PR-lenker og AI-metadata brukes fortsatt for å
finne relaterte elementer.

## Oppsett

Manglende konfigurasjon åpner veiledning for visningen. Enter prøver igjen,
`vs` åpner innstillingene, og visningsmenyen kan bytte side. j/k og
PageUp/PageDown ruller. Tekstfelt og oppsettdialoger eier inntastingen sin.
