# Forslag til nytt tastaturoppsett

Dette er et forslag til erstatning for dagens tastaturoppsett, ikke en beskrivelse
av tastene som er implementert. Dagens taster står i [keyboard.md](keyboard.md).
Alle eksisterende funksjoner skal fortsatt kunne utføres. Det som fjernes, er
muligheten til å utføre handlinger på skjulte relaterte elementer fra en annen side.

## Prinsipper

- Hurtigkommandoer bruker små bokstaver og piltaster, uten tall, symboler eller
  kombinasjoner med Shift/Ctrl/Alt.
- `h`, `j`, `k` og `l` brukes bare til navigering, også i menyer.
- `g` åpner sidemenyen, `a` åpner handlingsmenyen. Vanlige valg kan også utføres
  med to bokstaver, for eksempel `gg` eller `aw`.
- Menyene viser både navn og eventuell sekvens. Et valg uten egen sekvens er
  tilgjengelig med `j/k` eller piltaster og `y`.
- Bare handlinger som gjelder aktiv side, vises i handlingsmenyen.
- En valgliste for relasjoner eller koblingsmål gir ikke tilgang til handlinger
  på elementene som vises.
- Bokstaver i tekstfelt er tekst og starter aldri kommandoer. Vanlig tekstredigering
  er en egen modus; lagring og avbryting må vises tydelig i feltets veiledning.

## Grunntaster

| Tast | Funksjon |
| --- | --- |
| `j` / `↓` | Neste rad / rull ned |
| `k` / `↑` | Forrige rad / rull opp |
| `h` / `←` | Forrige fane |
| `l` / `→` | Neste fane |
| `d` | Detaljer; for chatmeldinger åpnes tråden |
| `f` | Søk i aktiv liste |
| `x` | Marker / fjern markering på aktiv rad |
| `v` | Start / avslutt områdemarkering; utvid med opp/ned |
| `u` | Fjern alle markeringer |
| `o` | Åpne valgt element i nettleseren |
| `e` | Åpne valgt elements arbeidsmappe i editor |
| `r` | Oppdater aktiv side eller detaljer |
| `w` | Vis relasjoner til valgte elementer |
| `c` | Velg elementer som skal kobles til valget |
| `g` | Sidemeny |
| `a` | Handlingsmeny for aktiv side |
| `b` | Hjelp for aktiv side |
| `y` | Velg / bekreft i menyer og dialoger |
| `q` | Avbryt / tilbake; på hovednivå åpnes bekreftelse på avslutning |

På oppgavesiden bytter `h/l` oppgave, og `j/k` navigerer mellom referanser når
oppgavedetaljene er åpne. I bruksstatistikk bytter `h/l` periode.
Kommandoene for nettleser og editor bruker bare det valgte elementets eget mål.
Elementer uten egnet mål gir en beskjed og åpner ikke et tilfeldig eller relatert
element. Bruk `w` for å gå til et relatert element først.

## Sider

| Sekvens | Side |
| --- | --- |
| `gt` | Oppgaver |
| `gg` | GitHub |
| `gi` | Jira |
| `ga` | AI / Copilot |
| `gc` | Chat / Mattermost |
| `gr` | Repositoryer med skrivetilgang |
| `gm` | Lokale mapper |
| `gz` | Utsatte |
| `gb` | Lokalt arkiv |
| `gs` | Innstillinger |
| `gu` | Lokal bruksstatistikk |
| `gd` | Copilot-diagnostikk |
| `go` | Oppsettsveiledning for aktiv side |
| `gp` | Forrige hovedside |
| `gv` | Sammenheng for valgte elementer |

`gd` bytter til diagnostikksiden. Kopiering eller andre handlinger på den siden
utføres først etter sidebyttet. `go` åpner relevant veiledning; manglende oppsett
kan også åpne veiledningen automatisk som i dag.

## Valg og mål

- Uten markering brukes raden som har fokus. Med markering brukes bare synlige,
  markerte rader i aktiv liste.
- Side- og fanebytte fjerner markeringer og avslutter områdemarkering.
- Et filter tar ikke med markerte rader som filteret skjuler. De kan vises igjen
  når filteret endres, men påvirkes ikke av handlinger mens de er skjult.
- Områdemarkering har et fast startpunkt og utvides eller trekkes tilbake når
  markøren flyttes. `v` avslutter modusen og beholder det synlige utvalget.
- Ved start av en handling låses mål og antall. En bakgrunnsoppdatering må ikke
  flytte handlingen til en annen rad. Valgte mål vises i dialogen.
- Detaljer og tittelredigering gjelder ett aktivt element. Flervalg beholdes for
  handlingene som støtter flere elementer i dag.
- Sletting, tømming av chatkonto og avslutning bekreftes med `y`; `q` avbryter.

## Felles handlinger

Disse menyvalgene finnes på sidene der de er relevante:

| Sekvens | Menyvalg | Virkning |
| --- | --- | --- |
| `aa` | Arkiver lokalt | Skjul valgte elementer uten å endre eksterne systemer |
| `az` | Utsett / endre utsettelse | Åpne dialog for varighet og returtidspunkt |
| `as` | Send til chat | Del valgte elementer til konfigurert webhook-kanal |
| `ag` | Gjenopprett | Gjenopprett valgte oppføringer fra Arkiv eller Utsatte |
| Via `a` | Vis eksterne lenker | Vis søkbar liste over lenker i valgte elementer |

Sending beholder dagens innhold, mottaker og feilhåndtering: én melding per PR,
repoets visningsnavn først i lenketeksten, og ny sending av usendte meldinger etter
feil. Chatkanalen til en valgt melding overstyrer ikke webhook-mottakeren.

Utsettelsesdialogen tilbyr forhåndsvalg og justering av antall/enhet med piltaster.
Alle varigheter som støttes i dag skal kunne velges, inkludert timer,
kalenderdager og kl. 06 etter ønsket antall kalenderdager. Ingen talltaster er
nødvendige som kommandoer. Fra Utsatte kan tidspunktet endres og én eller flere
oppføringer gjenopprettes. Lagrede detaljer kan fortsatt åpnes uten å velge en
annen rad.

## Handlinger per side

### Oppgaver

| Sekvens | Menyvalg |
| --- | --- |
| `an` | Ny oppgave: tom eller fra medbrakte elementer |
| `ae` | Endre aktiv oppgaves tittel |
| `ad` | Slett valgte oppgaver; i oppgavedetaljer: fjern valgte referanser |

Oppgavesletting fjerner også tilhørende lokale koblinger. Referansesletting
sletter ikke de refererte PR-ene, Jira-sakene, AI-sesjonene eller chatmeldingene.

### GitHub

| Sekvens | Menyvalg |
| --- | --- |
| `am` | Automerge: velg på eller av |
| `ar` | Be om ny Copilot-review |
| `ad` | Slett lokale worktrees for valgte PR-er |
| `av` | Velg editor: IntelliJ, VS Code eller egendefinert |

`e` klargjør og åpner PR-ens worktree i standardeditor. Sletting gjelder lokale
worktrees, ikke PR-er eller repositories. Dagens kontroll av lokale endringer
og usporede filer beholdes.

### Jira

| Sekvens | Menyvalg |
| --- | --- |
| `at` | Velg statusovergang |
| `am` | Tilordne meg |
| `af` | Fjern min tilordning |
| `aw` | Velg repo og opprett arbeidsmappe med branch |
| `ap` | Rediger hurtigfiltre |
| `ae` | Rediger egen JQL |
| `ab` | Lagre aktivt søk |
| `au` | Nullstill hurtigfiltre og egen JQL |

Statusdialogen viser faktiske, entydige overganger som er felles for valgte saker.
QA, under arbeid, avklaring og ferdig kan fortsatt velges når arbeidsflyten
støtter dem. Alle andre støttede overganger beholdes. Feil stopper resten av
flervalget; allerede utførte endringer kan ikke avbrytes med `q`.

`af` fjerner bare egen tilordning, ikke andre ansvarlige.

`aw` krever én sak og viser eksisterende repoer med skrivetilgang. `y` bekrefter
repovalget og oppretter eller gjenbruker sakens branch/worktree. `e` bruker samme
flyt og åpner editor etterpå. Repovalget er et argument til Jira-handlingen og
gir ikke tilgang til repositoryhandlinger. Å opprette et helt nytt repository
på GitHub er ikke en eksisterende funksjon og inngår ikke i dette forslaget.

### AI / Copilot

`e` åpner en målvelger med «Chattens arbeidsmappe» og «Worktreeroten». Dagens
fallback for manglende lokale mapper beholdes. `d` viser sesjonsdetaljer.
Arkivering, utsettelse, sending, koblinger og relasjoner beholdes.

Copilot-diagnostikk åpnes med `gd`. Handlingsmenyen der inneholder
«Kopier OTLP-oppsett». Det er ikke en kommando på andre sider.

### Chat / Mattermost

| Sekvens | Menyvalg |
| --- | --- |
| `ai` | Innlogging: passord med eventuell TOTP, eller PAT |
| `ac` | Velg kanaler som skal overvåkes |
| `am` | Merk aktiv kanal lest |
| `av` | Slutt å overvåke aktiv kanal |
| `ad` | Tøm lokal konto og lagret sesjon |
| `ae` | Hent eldre meldinger |
| Via `a` | Rull en halv side opp |
| Via `a` | Rull en halv side ned |

`d` åpner tråden, `o` åpner meldingen i nettleseren, og `h/l` bytter kanal/fane.
Bekreftelsen for kontotømming viser hva som fjernes lokalt.

### Mapper og repositoryer

| Side | Sekvens | Menyvalg |
| --- | --- | --- |
| Mapper | `af` | Slå favoritt av/på |
| Mapper | `av` | Skjul / vis mappe igjen |

Mapper beholder fanene Alle, Favoritter og Skjulte. `e` åpner valgt mappe,
`f` søker og `r` oppdaterer. På repositorysiden søker `f`, `o` åpner valgt repo
på GitHub og `r` oppdaterer listen over repoer med skrivetilgang.

### Innstillinger, oppsett og statistikk

- Innstillinger: `j/k` velger felt, `y` åpner det for redigering. Alle eksisterende
  innstillinger, validering og lagringsmuligheter beholdes.
- Oppsett: veiledning, utfylling, validering, oppstartskø for manglende
  integrasjoner og ny tilkoblingsprøve beholdes. Menyvalg for «Fyll ut oppsett»
  og «Prøv tilkoblingen igjen» skal være synlige.
- Statistikk: `h/l` bytter 7 dager / 30 dager / alt, `j/k` ruller, `q` går tilbake.
- Avslutning: `q` på hovednivå viser bekreftelse og avslutter med kansellering av
  bakgrunnsarbeid og lagring av lokale endringer som i dag.

## Relasjoner, kildekontekst og samlet utsettelse

### Gå til relaterte elementer

`w` viser en søkbar liste over relaterte elementer, med synlig type og nøkkel.
`y` går til valgt elements egen side. Listen tilbyr navigering, søk og detaljer,
men ingen endringer av de relaterte elementene.

Eksempel: Fra en PR åpner `w` Jira-saken. Etter sidebyttet kan `at` endre status
eller `aw` opprette en arbeidsmappe. Gamle kjeder som `atfq` utfører ingen handling.

### Koble elementer

`c` åpner målvelgeren. Typevalg, søk og flervalg finnes i denne flyten. `y`
bekrefter koblingene lokalt. Alle valgte kilder kan fortsatt kobles til alle
valgte mål, uten selvkoblinger eller duplikater. Toveiskoblinger og referanser
på interne oppgaver beholdes.

### Opprette oppgave fra andre sider

Ved navigering til Oppgaver med `gt` tas valgte kildeelementer med som en
uttrykkelig kildekontekst. Ingen oppgave opprettes ved sidebyttet. Kilden og
antallet vises på oppgavesiden, og `an` tilbyr «Tom oppgave» eller «Oppgave fra
medbrakte elementer». Brukeren kan forkaste kilden. Oppgaven og referansene
opprettes først etter valg på oppgavesiden.

### Utsette hele sammenhengen

`gv` åpner en egen sammenhengsvisning med de valgte røttene og alle berørte
elementer synlige. Den beholder dagens avgrensning av sammenhengen, inkludert
beskyttelsen mot å ta med uvedkommende oppgaver gjennom en delt PR.

Visningen har bare lokale organiseringshandlinger. `az` åpner utsettelse av den
viste sammenhengen. Den gir ikke tilgang til Jira-status, GitHub-automerge eller
andre eksterne handlinger på tvers av typer. Gruppens medlemmer, returtidspunkt
og samlet gjenoppretting beholdes etter omstart, også når en integrasjon er
utilgjengelig.

## Kontrollpunkter ved implementering

1. Alle funksjonene over er tilgjengelige fra både relevante lister og detaljer.
2. Hjelp, bunnlinjer, README og tastaturoversikt viser samme oppsett som ruteren.
3. Ingen gammel handlingskjede kan endre et relatert element uten sidebytte.
4. Filtrerte eller andre siders markeringer påvirker ikke aktiv handling.
5. Bakgrunnsoppdateringer, avbrudd og feil endrer ikke handlingens låste mål.
6. Tekst i søk, JQL, innlogging, oppsett og titler starter aldri kommandoer.
7. Flervalg, lokal lagring, delvise feil og eksisterende sikkerhetskontroller
   fungerer fortsatt for alle integrasjoner.
8. Bruksstatistikken teller de nye kommandoene med faste navn uten å lagre
   tekstfeltinnhold, nøkler, titler eller andre sensitive målverdier.
