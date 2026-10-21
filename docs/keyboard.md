# Tastaturnavigasjon

Dette er den fullstendige oversikten. Tastene gjelder i navigasjon, også i
read-only detaljer. Tekstfelt og dialoger eier inntastingen sin. Enter bekrefter
et valg eller et tekstfelt; den åpner ikke lenger elementer fra hovedlisten.
Gamle enkelttaster og tall-/Tab-snarveier er fjernet.

## Tastene fra spesifikasjonen

| Tast | Handling |
| --- | --- |
| `j` / `k` / `↓` / `↑` | Neste/forrige rad; rull i detaljer |
| `h` / `l` / `←` / `→` | Forrige/neste fane; på oppgavesiden forrige/neste oppgave |
| `d` | Detaljer; for en chatmelding åpnes tråden |
| `/` | Søk eller filtrer i aktiv visning |
| `?` | Vis tilgjengelige hurtigtaster |
| `vh` | Oppsettshjelp for aktiv side; vises automatisk når en visning mangler oppsett |
| `Space` | Marker eller fjern markering av aktiv rad |
| `Shift+↑` / `Shift+↓` | Utvid markeringen med aktiv og neste/forrige rad |
| `x` | Fjern alle markeringer uten å endre søk eller filtre |
| `z` + valgfritt tall + `h` / `d` | Utsett valgte elementer; uten tall brukes 1 |
| `zz` + valgfritt tall + `h` / `d` / `m` / `D` | Vis og utsett hele sammenhengen; `zzh` betyr én time |
| `Esc` | Avbryt sekvens/dialog, lukk detaljer eller fjern markeringer |

Flervalg finnes for oppgaver, PR-er, Jira-saker, AI-sesjoner, chatmeldinger,
oppgavereferanser og valglister. Markeringer beholdes ved bytte av fane/filter,
men markeringer i en annen type eller en annen oppgaves referanser tas ikke med
som et skjult valg. Uten markering bruker handlinger den aktive raden.
Detaljer og tittelredigering gjelder ett aktivt element.

| Ting | Tast |
| --- | --- |
| Interne oppgaver | `o` |
| GitHub / Git management | `g` |
| Jira / task management | `t` |
| AI / Copilot | `a` |
| Chat / Mattermost | `c` |

| Starttast | Mønster | Handling |
| --- | --- | --- |
| `r` | `ro`, `rg`, `rt`, `ra`, `rc` | Relaterte elementer av valgt type |
| `v` | `vo`, `vg`, `vt`, `va`, `vc` | Visningsside |
| `t` | `to`, `tg`, `tt`, `ta`, `tc` | Velg elementer som skal kobles til valget |
| `e` | `ee`, `en`, eller `e` + ting … + `e`/`n` | Åpne i editor / nettleser |
| `a` | `a` + eventuelle ting … + handling | Utfør handling på valget/relasjonen |

Ved kobling vises lastede elementer og lagrede referanser av valgt type.
Velg med j/k, Space eller Shift+piltaster, og bekreft med Enter. Alle valgte
kilder kobles til alle valgte mål, uten selvkoblinger eller duplikater. Koblingene
er lokale og toveis, og lagres i `links.sqlite` i konfigurasjonsmappen
(`CUSTOM_VISNING_LINKS` kan overstyre plasseringen). En oppgave får også
referansene lagt til i sin egen lagring. Eksisterende oppgavereferanser,
Jira-nøkler, PR-lenker i meldinger og AI-sesjonenes Jira-/repo-/branchdata brukes
som naturlige relasjoner. Gamle oppgaver får en stabil intern ID ved behov.

Ett relatert treff åpnes direkte; flere treff vises i en søkbar valgliste.
Fra listen kan man markere flere og fortsette med `a`, `e`, `r` eller `t`.
Enter går til aktivt treff. Hver ting i en sekvens følger én relasjon; en handling
utvides aldri til et helt nettverk av indirekte sammenkoblede oppgaver.

## Handlinger og kjeding

| Handling | Tast etter `a` |
| --- | --- |
| Send meldinger til konfigurert Mattermost-webhook | `s` |
| Arkiver valgte elementer lokalt / gjenopprett fra arkivvisningen | `k` |
| Slett intern oppgave eller valgte referanser i oppgavedetaljer | `d` |
| Flytt Jira-saker: hent og velg faktisk overgang | `f` |
| Tilordne Jira til meg | `m` |
| Fjern min Jira-tilordning, uten å fjerne andre ansvarlige | `r` |

`as` sender valgte elementer direkte, med én melding per PR og repoets
visningsnavn først i lenketeksten. Ingen editor åpnes, og Jira-lenker legges ikke
til som egne linjer. Ved feil beholdes usendte meldinger; `as` prøver disse igjen.
Mottakeren er den konfigurerte webhook-kanalen, ikke automatisk kanalen til en
markert chatmelding.

`ak` skjuler elementene fra vanlige lister uten å endre eksterne systemer.
`vb` viser det lokale arkivet, der `ak` gjenoppretter valgte elementer.
`d` viser lagrede detaljer om arkiverte/utsatte elementer uten å velge en annen rad.
Utsatt-visningen støtter også søk, flervalg og samlet gjenoppretting med `az`.
`ad` sletter ikke PR-er, Jira-saker, AI-historikk eller chatmeldinger eksternt.
Ved oppgavesletting fjernes også tilhørende lokale koblinger.

| Tast etter Jira-flytting (`af` eller `atf`) | Målstatus |
| --- | --- |
| `q` | QA |
| `u` | Under arbeid |
| `a` | Avklaring |
| `f` | Ferdig |

Overgangene hentes fra hver saks arbeidsflyt. Navn som QA/Quality Assurance,
In Progress/Under arbeid, Clarification/Avklaring og Done/Closed/Ferdig støttes.
Listen viser bare entydige målstatuser som er felles for alle valgte saker.
Velg en annen tilgjengelig overgang med j/k og Enter. En snarvei med manglende
eller tvetydig status utfører ingen endring. Jira kan avvise overganger som
krever flere felt. Ved feil stoppes resten av flervalget; tidligere saker kan
allerede være oppdatert. Esc avbryter før utføring, ikke en allerede sendt endring.

Eksempler:

- `atfq` fra GitHub: Jira-sakene til valgte PR-er flyttes til QA, uten visningsbytte.
- `atfu`, `atfa`, `atff`: samme målvalg, andre statuser.
- `atm` / `atr`: tilordne meg / fjern meg fra tilhørende Jira-saker.
- `afq` / `am` / `ar` direkte i Jira: handling på valgte saker.
- `etn`: tilhørende Jira-saker åpnes i nettleseren.
- `eae`: tilhørende AI-sesjoners prosjekt åpnes i IntelliJ.
- `atgs`: følg Jira, så GitHub, og lag melding om PR-ene som ble funnet.
- `aod`: slett den tilhørende interne oppgaven.
- `to` fra AI/chat/Jira/GitHub: velg intern oppgave som skal få referansene.

Målene tas fra valget når sekvensen starter. Bakgrunnsoppdateringer flytter ikke
handlingen til en annen rad. QA-tasten kan også skrives mens overgangene lastes.
`ee` bruker IntelliJ. I AI-visningen venter `ee` på tredje tast:
`eee` åpner chattens arbeidsmappe (`cwd`), mens `eew` åpner Git-/worktreeroten
(`gitRoot`). Hvis den foretrukne mappen mangler eller ikke finnes lokalt, brukes
prosjektroten og deretter den andre mappen. Esc avbryter uten å åpne editoren.
I andre visninger og i relasjonskjeder som `eae` virker `ee`/editorhandlingen som før.
Fra typer uten eget prosjekt følges relasjoner til lastede
PR-er og AI-prosjekter. `en` åpner HTTPS-adresser; AI-sesjoner uten nettadresse
kan bare åpnes i editor. En referanse som ikke er lastet kan ikke åpnes i editor
ved å falle tilbake til en tilfeldig aktiv PR.

## Valg som ikke var definert i listen

Følgende utvider mønsteret for funksjoner som allerede fantes:

| Sekvens | Betydning |
| --- | --- |
| `au` | Oppdater/synkroniser aktiv visning eller detaljer; prøv GitHub-oppstart igjen |
| `an` | Ny intern oppgave fra valgte elementer; på oppgavesiden en tom oppgave |
| `ae` | Endre aktiv oppgaves tittel; `aoe` kan først finne en tilhørende oppgave |
| `aq` / `ax` | Lagre aktivt Jira-søk / nullstill Jira-hurtigfiltre og egen JQL |
| `vs` / `vr` | Innstillinger / forrige hovedvisning |
| `vm` | Lokal bruksstatistikk; h/l velger siste 7 dager, siste 30 dager eller alt |
| `vi` | Copilot-diagnostikk |
| `vf` / `vj` | Jira-hurtigfiltre / egen JQL |
| `vz` / `az` | Vis utsatte / gjenopprett valgte utsatte elementer |
| `vb` | Vis lokalt arkiv |
| `vk` | Velg hvilke Mattermost-kanaler som skal overvåkes |
| `al` / `ap` | Chat-innlogging med passord/TOTP / PAT |
| `av` / `aw` / `ab` | Merk kanal lest / slutt å overvåke / tøm lokal chatkonto og lagret sesjon |
| `ah` / `a↓` / `a↑` | Eldre chatmeldinger / rull en halv side ned/opp |
| `el` | Vis en liste over eksterne lenker i valgte elementer |
| `ev` | Velg editor med j/k + Enter: IntelliJ, VS Code eller egendefinert |
| `z` + valgfritt tall + `m` / `D` | Utsett til kl. 06 etter antall kalenderdager; uten tall brukes 1 |
| `Ctrl+C` | Avslutt og kanseller bakgrunnsarbeid |
| `Enter` | Bekreft valg/tekst i dialoger; ikke en snarvei i hovedlisten |

`a` og `c` velger AI og chat i kjeder. `s` avslutter med melding og `k` med
arkivering. For eksempel følger `aas` AI og lager melding om sesjonene, mens
`acs` følger chat og lager melding om treffene. `aak` og `ack` arkiverer de
tilhørende AI-sesjonene og chatmeldingene. Det trengs ikke kolon i sekvensene.

Listen definerte heller ikke om kobling, arkivering og sletting skulle endre
systemene eksternt, hvilken chatkanal en melding skulle sendes til, hvordan
flertydige relasjoner skulle velges, eller hva som skulle skje når en valgt type
mangler data/konfigurasjon. Her er valgene: koblinger/arkiv er lokale, sletting er
avgrenset til interne oppgaver/referanser, meldinger bruker konfigurert webhook,
flere treff gir valgliste, og manglende mål gir en synlig melding uten å velge et
annet objekt. Ingen nye API-er for ekstern sletting, fjernarkivering eller
endring av eksterne relasjoner er innført.

## Oppsettshjelp

Når en visning mangler konfigurasjon, åpnes en veiledning med steg i appen og
utenfor appen. `vs` åpner relevante innstillinger; Esc returnerer til veiledningen.
Enter prøver å åpne siden igjen. `vg`/`vt`/`va`/`vc` og `vo` bytter side, og
j/k, piltaster eller PageUp/PageDown ruller veiledningen. Ctrl+C avslutter.

Mattermost-server og brukernavn kan lagres i innstillingene og tas i bruk med
Enter eller `vc` ved første oppsett. GitHub/Jira-token og webhook krever omstart
fra terminalen der innloggingen eller miljøvariablene ble satt.

## Lokale mapper

| Tast | Handling |
| --- | --- |
| `vl` | Vis lokale mapper som appen kjenner |
| `h/l` | Alle / Favoritter / Skjulte |
| `/` | Søk i mapper eller i repositoryvalget |
| `af` | Slå favoritt av/på for valgt mappe |
| `ah` | Skjul / vis valgt mappe igjen |
| `ee` | Åpne valgt mappe i standardeditor |
| `au` | Oppdater mappeoversikten |
| `aw` eller `ee` på én Jira-sak | Velg repo med skrivetilgang og opprett eget worktree |
| `Enter` i repositoryvalget | Opprett mappe fra valgt Jira-sak og repo |
| `Esc` | Lukk mappevisningen eller avbryt repositoryvalget |

Skjulte mapper vises i fanen Skjulte, der `ah` henter dem tilbake. Valg og
Jira-koblinger lagres lokalt. Opprettelsen kontrollerer skrivetilgangen på nytt
og bruker en egen branch fra `workspace.main_branch`.
