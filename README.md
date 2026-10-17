# custom-visning

Et k9s-inspirert GitHub PR- og Jira-dashboard i terminalen. Se egne åpne PR-er, drafts,
direkte review requests og teamforespørsler på tvers av alle repositories du har
tilgang til, inkludert private repositories og flere organisasjoner.

```text
 CUSTOM VISNING GitHub · alice
[1 My Open PRs]  2 My Draft PRs  3 My Review Requests  4 Team Review Requests
 ready · last full sync: 2026-10-17 10:35:02
REPOSITORY           PR      TITLE                       AUTHOR       REVIEW    CI      UPDATED
acme/frontend        #123    Fix login flow              alice        APPROVED  PASS    2h
acme/backend         #456    Add new endpoint            alice        PENDING   FAIL    5h
 1/2 · merge: MERGEABLE · reviewers: bob · teams: acme/core · labels: bug
1–4 views • j/k ↑/↓ move • Enter open • r refresh • / filter • ? help • q quit
```

## Krav og oppstart

- Go **1.27.2 eller nyere**, som angitt i `go.mod`.
- En terminal med interaktiv TTY. Farger tilpasses terminalens støtte.
- GitHub.com-konto med tilgang til repositories som skal vises.
- GitHub CLI (`gh`) med eksisterende innlogging, eller `GITHUB_TOKEN`.

```bash
git clone https://github.com/haakonph/custom-visning-terminal.git
cd custom-visning-terminal
gh auth login --hostname github.com
go run ./cmd/custom-visning
```

Programmet henter token **én gang ved oppstart**: først `GITHUB_TOKEN`, deretter
`gh auth token --hostname github.com`. Alle datakall gjøres direkte fra Go med en
gjenbrukbar HTTP-klient; `gh` brukes ikke til API-søk. Et eksplisitt ugyldig
`GITHUB_TOKEN` faller ikke tilbake til en annen identitet. Token, CLI-stderr og
rå API-feilbodyer vises eller lagres aldri.

```bash
make build
./bin/custom-visning
./bin/custom-visning --version
# Alternativt:
go install ./cmd/custom-visning
```

## Installere release på macOS Apple Silicon

Last ned `custom-visning_Darwin_arm64.tar.gz` og `checksums.txt` fra
[Releases](https://github.com/haakonph/custom-visning-terminal/releases).
Fra mappen med de nedlastede filene:

```bash
shasum -a 256 -c checksums.txt
tar -xzf custom-visning_Darwin_arm64.tar.gz
mkdir -p "$HOME/.local/bin"
install -m 755 custom-visning "$HOME/.local/bin/custom-visning"
export PATH="$HOME/.local/bin:$PATH"
gh auth login --hostname github.com
custom-visning
```

Legg PATH-linjen i `~/.zshrc` for senere økter. Første versjon distribuerer kun
macOS arm64. Binæren er ikke Apple-signert eller notarized; følg macOS sin
vanlige godkjenning for nedlastet programvare ved behov.

## Visninger og hurtigtaster

| Tast | Handling |
| --- | --- |
| `1` | My Open PRs: egne åpne PR-er, uten drafts |
| `2` | My Draft PRs: egne åpne drafts |
| `3` | My Review Requests: bare direkte forespørsler til brukeren |
| `4` | Team Review Requests: forespørsler til tilgjengelige team brukeren er medlem av |
| `j` / `k`, `↓` / `↑` | Velg rad |
| `Enter` | Åpne valgt PR i standardnettleseren |
| `r` | Oppdater aktiv visning; prøv autentisering på nytt hvis oppstart feilet |
| `/` | Filtrer lokalt på repository, PR-nummer, tittel, forfatter, label, team eller status |
| `Enter` i filter | Behold filter og gå tilbake til navigasjon |
| `Esc` | Fjern filter og lukk hjelp |
| `?` | Vis/skjul hjelp |
| `q` / `Ctrl+C` | Avslutt og kanseller nettverksarbeid |

Filteret gjelder også ved bytte av visning. Oppdatering er asynkron og én
oppdatering per visning kan kjøre om gangen; fire visninger kan oppdateres
parallelt. Alle fire oppdateres ved oppstart, deretter manuelt med `r`.
Teamforespørsler dedupliseres, og teamnavn vises i detaljlinjen for valgt PR.
En PR som har både direkte og teamforespørsler kan naturlig forekomme i begge
visningene; teamtreff alene gir aldri treff i den direkte visningen.

Kolonner prioriteres etter vindusbredde. Smale vinduer viser PR-nummer og tittel;
større vinduer viser også repository, forfatter, review, CI og alder. Utvalgte
PR-ers labels, reviewers, team og mergeability står under tabellen. `UNKNOWN`
betyr at GitHub ikke ga noen kjent status, f.eks. ved manglende checks eller
reviewregel. `PENDING` betyr ventende review/checks, og `CHANGES` betyr at
endringer er etterspurt. Mergeability betyr konfliktdeteksjon, ikke at alle
branch protection-regler er oppfylt.

## GitHub-rettigheter

For et klassisk PAT / GitHub CLI OAuth-token:

- `repo` for private repositories og PR-data.
- `read:org` for `/user/teams`, inkludert private teammedlemskap.
- Autoriser tokenet for organisasjonens SAML SSO når organisasjonen krever det.

```bash
gh auth refresh --hostname github.com -s repo -s read:org
```

Et fine-grained PAT må ha tilgang til de aktuelle repositories og minst lesing
av metadata, pull requests, commit statuses og checks, samt organisasjonens
**Members: read** for team. Organisasjonens godkjenning kan være nødvendig.
Slike tokens er vanligvis begrenset til én ressurseier; bruk et CLI/OAuth-token
med nødvendige scopes for team på tvers av flere organisasjoner. GitHub returnerer
bare data identiteten faktisk kan lese. Team som API-et skjuler kan ikke listes
eller gi en eksplisitt feil per team. HTTP 401/403, SSO- og rate-limit-problemer
vises som forståelige statusmeldinger; eksisterende data beholdes.

## Cache, nettverk og begrensninger

Cachen ligger i `filepath.Join(os.UserCacheDir(), "custom-visning")`:

- macOS: `~/Library/Caches/custom-visning/`
- Linux: `$XDG_CACHE_HOME/custom-visning/`, normalt `~/.cache/custom-visning/`

Hver autentisering har en egen JSON-fil, navngitt med et SHA-256-fingeravtrykk.
Selve tokenet lagres ikke. Filen inneholder verifisert brukernavn, PR-data og siste
fullførte synkronisering per visning. Tokenbytte gir ny cache. Første oppstart
krever nettverk for identiteten. Senere oppstarter viser cache umiddelbart, også
uten nettverk, mens nye data hentes i bakgrunnen. Manglende autentisering krever
fortsatt innlogging. Korrupte/ukjente cacheversjoner ignoreres med en melding.

Cachemapper opprettes med `0700`, filer med `0600`. Cachen inneholder privat
PR-metadata i klartekst: slett cachemappen for å fjerne lokale data. Skriving skjer
via en midlertidig fil i samme mappe, `fsync` og atomisk rename. Skriving er
serialisert og samles opp for å unngå at eldre svar overskriver nyere snapshots.

REST Search API brukes til oppdagelse med alle sider frem til GitHubs grense på
**1 000 treff per søk**, også per team. Søket bruker det verifiserte brukernavnet
istedenfor `@me`, `sort=updated` og `order=desc`. Teammedlemskap pagineres på tvers
av organisasjoner. Direkte forespørsler bruker `user-review-requested:LOGIN`;
team bruker `team-review-requested:ORG/SLUG`.

GraphQL `nodes(ids:)` beriker opptil 40 PR-er per kall med draft-status,
reviewDecision, requested reviewers/teams, siste commits statusCheckRollup og
mergeable. Ingen ubetingede HTTP-kall per PR. Opptil 100 reviewer-forespørsler per
PR hentes; større lister markeres delvise. Ved GraphQL-/tilgangsfeil beholdes
søkeresultatene med eksplisitt ukjente statuser og advarsel. Resultater sorteres
lokalt etter sist oppdatert og dedupliseres på URL.

Ufullstendige søk, feil på en senere side, utilgjengelige team eller ufullstendig
berikelse markeres som delvise. Cachede rader beholdes, og tidspunktet for siste
fullstendige synkronisering flyttes ikke. Dette kan inkludere PR-er som siden er
lukket; en senere fullstendig oppdatering fjerner dem. GitHubs søkeindeks kan være
forsinket. Store teammedlemskap gir flere serielle søk og bruker søkekvoten.

Klienten har 10 sekunders connect-timeout, 20 sekunders header-timeout og 30
sekunders timeout per HTTP-request. En visningsoppdatering har en samlet frist på
to minutter. Midlertidige nettverksfeil, HTTP 429/5xx og eksplisitte rate-limit-403
forsøkes opptil tre ganger med backoff. `Retry-After` (sekunder/dato) og
`X-RateLimit-Reset` respekteres; ventetid over fem sekunder returneres som feil i
stedet for å holde oppdateringen i gang. HTTP 401 og permanente klientfeil
forsøkes ikke igjen automatisk. Alle ventinger kan kanselleres.

GitHub-integrasjonen støtter bare GitHub.com. Jira Data Center 10 støttes separat, se nedenfor. TUI-en kjenner bare det
SDK-uavhengige `model.Source`-grensesnittet, slik at andre tjenester kan kobles på
uten å flytte datainnhenting inn i renderingen. Ingen database eller webserver.

## Utvikling, tester og CI

```bash
make build
make run
make test  # vanlige tester + race detector
make lint  # go vet + formatteringskontroll
make clean
```

Testene bruker standardbiblioteket, `httptest` og lokale fakes. De trenger verken
nettverk, `gh`-innlogging eller ekte token. De dekker autentiseringsprioritet,
filtre, paginering, team, mapping, deduplisering, cache/korrupt cache,
API-feil/retry, timeout/cancellation, TUI-navigasjon, filter og refresh.

CI kjører separate jobber for test/race, vet/gofmt og build ved push til `main`,
PR-er mot `main` og `merge_group`. Utdaterte PR-kjøringer kanselleres; merge queue
kanselleres ikke av concurrency-regelen. CI har bare `contents: read` og ingen
hemmeligheter. Go-versjonen hentes fra `go.mod`, med dependency/build-cache.

Release-workflowen kjører ved push av `v*`-tagger og tester den taggede commiten
før bygging. Den cross-kompilerer macOS arm64 uten CGO, bygger inn taggen i
`--version`, lager arkiv og SHA256-checksums og publiserer GitHub Release med
workflowens `GITHUB_TOKEN`. Bare publiseringsjobben har `contents: write`.
Opprett og push en versjonstagg når en release er ønsket, for eksempel:

```bash
git tag v0.1.0
git push origin v0.1.0
```

Dependabot sjekker Go-moduler og GitHub Actions ukentlig og grupperer kompatible
minor-/patch-oppdateringer.

## API-referanser

- [Bubble Tea v2 upgrade guide](https://github.com/charmbracelet/bubbletea/blob/main/UPGRADE_GUIDE_V2.md)
- [go-github](https://github.com/google/go-github)
- [GitHub PR-søk](https://docs.github.com/en/search-github/searching-on-github/searching-issues-and-pull-requests)
- [Teammedlemskap](https://docs.github.com/en/rest/teams/teams#list-teams-for-the-authenticated-user)
- [GraphQL PullRequest](https://docs.github.com/en/graphql/reference/objects#pullrequest)

## Jira Data Center 10

Jira bruker REST API v2 med et personlig tilgangstoken (PAT), hvis administratoren
har tillatt det. Alle Jira-kall er lesing, også POST til søkeendepunktet.
Sett serveradresse og token i miljøet. HTTPS er påkrevd; vanlige systemsertifikater
brukes. Jira under en context path, for eksempel `/jira`, støttes.

```bash
export JIRA_BASE_URL="https://jira.example.no/jira"
# Sett JIRA_TOKEN gjennom din vanlige hemmelighetshåndtering.
export JIRA_PROJECT="APP"  # valgfritt standard prosjektfilter
custom-visning --jira
# Fra kildekoden:
go run ./cmd/custom-visning --jira
```

`--jira` starter direkte med Jira og krever ingen GitHub-innlogging. Hvis GitHub
allerede er konfigurert med `GITHUB_TOKEN` eller `gh auth login`, er begge
visninger tilgjengelige også ved oppstart med `--jira`. Alle konfigurerte tjenester
er tilgjengelige i samme kjøring; flagget velger bare startvisningen. Uten
GitHub-innlogging starter appen automatisk i Jira dersom Jira er konfigurert.
Ugyldig Jira-konfigurasjon rapporteres uten å hindre oppstart av GitHub.

**Trykk `g` for GitHub og `J` (Shift+j) for Jira.** Hurtigtastene fungerer
i navigasjonsmodus. Mens du redigerer et filter eller JQL, skrives bokstavene
som vanlig tekst; avslutt redigeringen før du bytter. Begge visninger beholder
valgt fane, filter, markert rad og data. Byttet utløser ingen ny henting etter
første åpning; bruk `r` når du vil oppdatere. Snarveiene vises øverst i begge visninger.
GitHub-feil og Jira-feil håndteres hver for seg under kjøring.

Standardfanene er **Mine**, **Prosjekt**, **Uten ansvarlig** og **Aktiv sprint**.
Alle bruker `statusCategory != Done`. Prosjektfanen viser alle tilgjengelige
prosjekter når prosjektfilter ikke er satt. Aktiv sprint krever Jira Software.

| Tast | Jira-handling |
| --- | --- |
| `g` | Gå til GitHub; behold tilstanden |
| `J` (Shift+j) | Gå til Jira; behold tilstanden |
| `1`–`4` | Velg standardfane, som i GitHub-visningen |
| `5`–`9` | Velg de fem første lagrede søkene direkte |
| `Tab` / `Shift+Tab`, `←` / `→` | Bytt mellom standardfaner og lagrede søk |
| `j` / `k`, `↓` / `↑` | Velg sak |
| `Enter` | Åpne valgt sak i Jira |
| `/` | Filtrer lokalt blant hentede saker |
| `f` | Rediger kombinerbare hurtigfiltre i standardfaner |
| `:` | Rediger og kjør full JQL, inkludert ønsket `ORDER BY` |
| `s` | Lagre gjeldende JQL med navn; samme navn erstatter eksisterende søk |
| `x` | Fjern hurtigfiltre og midlertidig JQL; vis aktiv fanes søk |
| `r` | Hent aktivt søk på nytt |
| `Esc` | Avbryt redigering eller fjern lokalt tekstfilter |
| `?` | Vis/skjul hjelp |
| `q` / `Ctrl+C` | Avslutt og kanseller nettverksarbeid |

Fanene viser nummeret du kan trykke for å velge dem, på samme måte som i
GitHub-visningen. Lokalt tekstfilter beholdes ved fanebytte. Flere enn fem lagrede
søk nås med Tab eller piltastene.

Hurtigfiltrene er prosjekt, status, ansvarlig (Jira-brukernavn), prioritet,
sakstype, sprint, epic og label. Enter går til neste felt; tom verdi fjerner
filteret. Søket kjøres etter siste felt. Esc avbryter hele filterendringen.
Hurtigfiltre gjelder på tvers av standardfaner. Lagrede søk og egen JQL kjøres
uendret, slik at de også kan vise ferdige saker. Feltnavn som `Epic Link`
avhenger av Jira-oppsettet; bruk egen JQL dersom deres installasjon bruker andre
felt. Endre sortering gjennom `ORDER BY`, for eksempel `updated DESC` eller
`priority DESC, updated DESC`.

Tabellen viser saksnøkkel, prioritet, status, tittel, ansvarlig og alder.
På smale terminaler prioriteres saksnøkkel, status og tittel. Under tabellen vises
prosjekt, sakstype, ansvarlig, frist, labels og et kort utdrag av beskrivelsen.
Full beskrivelse finnes via Enter i Jira.

Lagrede søk ligger i `filepath.Join(os.UserConfigDir(), "custom-visning", "jira.json")`.
Bruk `JIRA_CONFIG` for å velge en annen fil. Tokenet lagres aldri i denne filen.
Slett eller endre søk ved å redigere filen og starte appen på nytt:

```json
{
  "queries": [
    {
      "name": "Mine feil",
      "jql": "assignee = currentUser() AND issuetype = Bug AND statusCategory != Done ORDER BY priority DESC"
    }
  ]
}
```

Søkeresultater caches under appens cachemappe i undermappen `jira`, adskilt per
server, token og full JQL med SHA-256-fingeravtrykk. Cachen inneholder sakens
metadata og beskrivelse i klartekst, med mapper `0700` og filer `0600`. Slett
undermappen for å fjerne lokale Jira-data. Ved oppstart/søk vises eventuell cache
mens data hentes. Feil beholder forrige resultat og flytter ikke siste vellykkede
synkronisering. Alle sider må lykkes før et nytt resultat erstatter cachen.
Jiras `ORDER BY` beholdes. Oppdateringer skjer ved oppstart, søk/fanebytte og `r`.
Nye søk kansellerer forrige søk, og forsinkede svar ignoreres.

Klienten har 30 sekunders request-timeout og to minutter per søk. Ved ugyldig JQL,
401/403, rate limit eller serverfeil vises en melding; rå feilbodyer og token
vises ikke. Redirects følges ikke, og nettleserlenker begrenses til konfigurert
Jira-server og context path. Integrasjonen endrer ingen Jira-saker.


## Navigering mellom GitHub og Jira

Trykk `l` i navigasjonsmodus for å vise relaterte saker eller PR-er.

- Fra GitHub: Jira-nøkler (for eksempel `TEAM-123`) hentes fra PR-tittel, beskrivelse og branchnavn. Jira viser alle refererte saker i et eget JQL-søk, uavhengig av hurtigfiltrene.
- Fra Jira: viser relaterte PR-er fra alle fire lastede GitHub-visninger, uten duplikater. Dette er ikke et søk i alle organisasjonens PR-er.
- `Enter` åpner valgt PR eller sak i nettleseren. `Esc` i GitHub fjerner avgrensningen til en Jira-sak. I Jira velger du en vanlig fane for å forlate relasjonssøket.
- `g` og `J` bytter fortsatt leverandør. Begge integrasjonene må være konfigurert for navigering begge veier.

Nøkler må bruke store bokstaver, og koblingen krever ikke skrivetilgang til GitHub eller Jira. Oppdater GitHub-data med `r` for å hente beskrivelser og branchnavn som mangler i gammel cache.


## Copilot-sesjoner

Terminalappen samler inn Copilot-data selv. Start med `custom-visning -copilot`
(fra kildekode: `go run ./cmd/custom-visning -copilot`), eller trykk **c** fra
GitHub/Jira. Ingen annen app, Node-server eller GitHub-token kreves for Copilot.
Direkteoppstart bruker bare Copilot; start på vanlig måte for å bytte mellom
øvrige konfigurerte integrasjoner.

Appen leser `workspace.yaml` og `events.jsonl` fra `~/.copilot/session-state`,
med de samme datakildene som Copilot-visningen i `custom-visning`. Den importerer
historikken ved oppstart og leser nye data hvert andre sekund. Samtale,
verktøyargumenter/resultater, feil, klient, modell, tokens og prosjekt vises når
kilden inneholder dem. Det lokale Copilot-filformatet er ikke en stabil offentlig
API; ukjente hendelser beholdes og ugyldige poster rapporteres i diagnostikk.
Kildemappene og filene endres aldri; symlinker følges ikke.

Terminalen har et eget datalager i `os.UserConfigDir()/custom-visning-terminal/copilot.json`
(på macOS normalt `~/Library/Application Support/custom-visning-terminal/copilot.json`,
på Linux normalt `~/.config/custom-visning-terminal/copilot.json`). Lagrede sesjoner,
inkludert mottatt telemetri, beholdes mellom oppstarter, også om kildefilene fjernes.
Filen erstattes atomisk med tilgang `0600`, og nye lagringsmapper opprettes med `0700`.
Lageret er separat fra både GitHub-cachen og den andre appens database.

### Egen OTLP-mottaker

Mens terminalappen kjører, mottar den også OTLP på `127.0.0.1:4318`, via
`/v1/traces`, `/v1/logs` og `/v1/metrics`. Både `http/json` og `http/protobuf`
støttes; protobuf bruker OpenTelemetry sine genererte skjemaer. Ingen telemetri
sendes videre. Mottakeren stopper når terminalappen avsluttes.

For Copilot CLI, eller en IDE som arver disse miljøvariablene:

```sh
export COPILOT_OTEL_ENABLED=true
export COPILOT_OTEL_CAPTURE_CONTENT=true
export OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318
export OTEL_EXPORTER_OTLP_PROTOCOL=http/json
# Start copilot eller IDE-en fra dette miljøet.
```

I IntelliJ kan tilsvarende endpoint settes i Copilot-innstillingene for CLI-agenten.
En IDE som allerede kjører, arver normalt ikke nye skallvariabler. Se Microsofts
[oppsett for Copilot CLI-sesjonsdata](https://github.com/microsoft/copilot-intellij-feedback/wiki/Access-Copilot-CLI-Session-Data)
og [OTel-veiledning](https://github.com/microsoft/copilot-intellij-feedback/wiki/Monitoring-Agent-Usage-with-OpenTelemetry).

Hvis en annen app bruker port 4318, fortsetter lokal filimport med en synlig
portfeil i diagnostikken. Sett for eksempel `COPILOT_OTLP_PORT=4319` for terminalen
og pek Copilots exporter til `http://127.0.0.1:4319`. Begge appene kan lese de samme
Copilot-filene uavhengig. En exporter sender til det endpointet du konfigurerer;
appene deler ikke telemetri eller lagringsfiler.

### Konfigurasjon og innhold

Disse miljøvariablene styrer terminalappen; start den på nytt etter endringer:

| Variabel | Standard | Bruk |
| --- | --- | --- |
| `COPILOT_SESSION_ROOT` | `~/.copilot/session-state` | Absolutt sti til historikkmappen; en egendefinert verdi må være ferdig ekspandert |
| `COPILOT_STORE_PATH` | Se datalagerstien ovenfor | Absolutt sti til terminalens eget JSON-lager |
| `COPILOT_OTLP_PORT` | `4318` | Lokal mottakerport |
| `COPILOT_COLLECTION_ENABLED` | `true` | `false` stopper filimport og mottaker; lagret historikk kan leses |
| `COPILOT_CAPTURE_CONTENT` | `true` | `false` utelater innhold, argumenter/resultater, feiltekst og rådata fra nye lagrede observasjoner |
| `COPILOT_SHOW_CONTENT` | `true` | `false` skjuler innhold, sesjonsnavn og rådata, også fra tidligere lagret historikk |
| `COPILOT_JIRA_PATTERN` | `[A-Z][A-Z0-9_]*-[1-9][0-9]*` | Jira-nøkler oppdages fra branch, metadata og brukerprompter |

Prompter, svar og verktøyresultater kan inneholde hemmeligheter. Innhold er på som
standard og lagres lokalt uten kryptering. Avslått innsamling eller innholdslagring
sletter ikke tidligere lagret innhold. Bruk visningsinnstillingen for å skjule det;
bruk et annet `COPILOT_STORE_PATH` for et separat lager. Innstillingene er uavhengige
av innstillingene i andre apper. `COPILOT_API_URL` brukes ikke.

Hendelser dedupliseres med stabil ID eller hash. Telemetri kobles via sesjons-ID og
trace-ID; poster uten slike ID-er vises separat. Senere sesjons-ID kan samle en
midlertidig `trace:…`-sesjon. Lokal `assistant.usage` prioriteres foran span- og
metric-bruk; overordnede spans utelates når underordnede har bruk, og kumulative
målinger summeres ikke som deltaer. Manglende verdier vises som ukjent.
Fullført, feil og venting krever en eksplisitt hendelse. Ellers utledes aktivitet
fra siste tidsstempel, med inaktiv etter to minutter.

JSONL-poster er begrenset til 2 MiB, workspace til 1 MiB og OTLP-forespørsler til
4 MiB (maks 20 000 normaliserte poster / 16 MiB). Komprimerte forespørsler støttes
ikke. Filfeil beholder tidligere importerte data og rapporteres uten kildeinnhold.

- **/** søker på sesjon, repository, prosjekt, Jira, klient, modell og status.
- **Enter** viser metadata, samtale, verktøyargumenter/resultater og feil.
- **i** åpner den valgte sesjonens prosjekt i IntelliJ fra listen eller detaljene.
- **j/k** eller pil opp/ned velger sesjon eller ruller i detaljene.
- **d** viser diagnostikk; **Esc** går tilbake og fjerner søket.
- **r** oppdaterer manuelt. Visningen oppdateres også hvert tredje sekund.
- **g** og **J** bytter til GitHub/Jira når de er konfigurert.

### Åpne IntelliJ fra en sesjon

Trykk **i** i Copilot-visningen. Appen bruker første eksisterende lokale mappe av
sesjonens prosjektrot, arbeidskatalog og Git-rot. IntelliJ åpner prosjektet eller
aktiverer det hvis det allerede er åpent. Dette åpner prosjektet; den konkrete
Copilot-chatten gjenåpnes ikke automatisk, siden sesjonsdataene ikke gir en støttet
IDE-lenke til den.

På macOS brukes `open -a "IntelliJ IDEA"`. På Linux brukes `idea`, og på Windows
`idea64.exe`, fra PATH. Med JetBrains Toolbox, Community Edition eller en annen
installasjonssti kan du sette `COPILOT_IDEA_COMMAND` til launcherens filsti, for
eksempel `COPILOT_IDEA_COMMAND="/path with spaces/idea"`. Verdien er ett kjørbart
program uten kommandolinjeflagg. Prosjektstien sendes som ett separat argument,
uten shell. Manglende prosjektmappe eller launcher vises som en melding i appen.
