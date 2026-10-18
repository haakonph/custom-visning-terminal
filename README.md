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
[siste release](https://github.com/haakonph/custom-visning-terminal/releases/latest).
Hver push til `main` publiserer automatisk en ny release etter beståtte sjekker.
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

## Laste ned bygg fra en branch

Alle brancher bygges ved push. Åpne [CI i GitHub Actions](https://github.com/haakonph/custom-visning-terminal/actions/workflows/ci.yml),
velg branchen i branch-filteret og åpne ønsket kjøring. Klikk **Last ned bygg**
i sammendraget for **Build**, eller velg `custom-visning-Darwin-arm64-<commit>`
under **Artifacts**. Du må være innlogget på GitHub for å laste ned artifacts.
Byggene beholdes i 30 dager.

Pakk ut artifact-ZIP-filen først. Den inneholder samme
`custom-visning_Darwin_arm64.tar.gz` og `checksums.txt` som en release;
følg installasjonstrinnene over. `custom-visning --version` viser
`build-<commit>`, slik at du kan kontrollere hvilket bygg du kjører.
PR-bygg bruker GitHubs test-merge-commit.

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

CI kjører separate jobber for test/race, vet/gofmt og build ved push til alle brancher,
PR-er mot `main` og `merge_group`. Utdaterte PR-kjøringer kanselleres; merge queue
kanselleres ikke av concurrency-regelen. CI har bare `contents: read` og ingen
hemmeligheter. Go-versjonen hentes fra `go.mod`, med dependency/build-cache.

Release-workflowen kjører ved push til `main` og ved push av `v*`-tagger.
Den tester commiten før bygging, cross-kompilerer macOS arm64 uten CGO og lager
arkiv og SHA256-checksums. Hvert main-bygg publiseres som siste GitHub Release med
en unik versjon `main-<kjøringsnummer>.<forsøk>-<commit>`. Taggede releaser bruker
taggen som versjon. Versjonen bygges inn i `--version`. Publisering bruker
workflowens `GITHUB_TOKEN`; bare publiseringsjobben har `contents: write`.
Du kan fortsatt opprette og pushe en versjonstagg, for eksempel:

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


## Oppgaver (fleksibelt konsept)

En oppgave er en lokal samling av eksplisitte referanser. Den kan være en tynn
wrapper rundt én Jira-sak eller PR, inkludert en PR fra review-fanene, eller samle
flere Jira-saker og PR-er på tvers av repoer. Den har egen tittel og trenger ingen
Jira-nøkkel. Endringer i oppgaven endrer ikke Jira eller GitHub.

- `t` i Jira/GitHub lager en oppgave fra valgt rad og åpner oppgavesiden.
- `T` åpner oppgavesiden; `j/k` velger oppgave i listen; `Tab` / `Shift+Tab` blar mellom oppgavene.
  På brede terminaler vises oppgavelisten til venstre og detaljene til høyre.
  På smale terminaler bytter du mellom liste og detaljer.
- `/` søker i oppgavetitler og tilknyttede referanser. `Enter` beholder søket,
  `Esc` nullstiller det.
- `Enter` / høyrepil går fra listen til referansene. Venstrepil / `Esc` går tilbake
  til listen. I detaljene velger `j/k` referanse og `Enter` åpner den.
- `g` / `J` går til GitHub/Jira. Finn en rad og trykk `a` for å legge den til
  oppgaven du sist valgte. Samme referanse legges ikke til to ganger.
- `n` lager en tom oppgave, `e` redigerer tittelen, `Enter` lagrer og `Esc` avbryter
  tittelredigeringen. En tom oppgave kan fylles ut senere.
- `d` fjerner valgt kobling.
  `D` sletter den valgte lokale oppgaven.
- `T` går tilbake til forrige provider-visning. `Esc` går først til listen,
  deretter nullstilles et eventuelt søk, og til slutt tilbake til provider-visningen.

Oppgavene lagres atomisk i `custom-visning/tasks.json` under operativsystemets
konfigurasjonsmappe (kan overstyres med `CUSTOM_VISNING_TASKS`). Filen er lokal for
OS-brukeren og deles mellom appens GitHub/Jira-kontoer; den inneholder titler og
lenker, ingen tokens. Ta vare på filen som brukerdata, ikke som cache.

Oppgavesiden bruker ferske titler, Jira-status og PR review/CI fra data som allerede
er lastet i appen. Ellers vises lagret referanse. Den henter ikke alle referanser
separat og utleder ingen samlet status eller automatisk gruppering. Dette holder
første versjon åpen for videre konseptarbeid; referansenes `provider`, `url`, `key`,
`title` og valgfri `role` er adskilt fra oppgavens egen tittel. Review-rollen settes
når PR-en legges til fra en review-fane og er oppgavens kontekst, ikke en live
reviewforespørsel.

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

## PR worktrees and editors

Press **Space** to toggle PR selection. Selection survives filters, tabs and
provider switches, so PRs from different repositories can be opened together.
Press **o** to open the selection in the default editor, or the highlighted PR
when nothing is selected. **O** opens a chooser: **1** IntelliJ IDEA, **2** VS
Code, **3** the configured custom command; **Esc** cancels. **Enter** continues
to open the PR in your browser. Editor choice in the chooser is for that
operation; set `editors.default` to persist your preference.

On first startup the app creates a JSON configuration file at
`os.UserConfigDir()/custom-visning/config.json` (normally
`~/.config/custom-visning/config.json` on Linux or
`~/Library/Application Support/custom-visning/config.json` on macOS).
Edit it and restart the application:

```json
{
  "repository_paths": ["/home/alice/src/frontend", "/home/alice/src/backend"],
  "editors": {
    "default": "intellij",
    "intellij": {"command": "idea", "args": ["{path}"]},
    "vscode": {"command": "code", "args": ["--new-window", "{path}"]},
    "custom": {"command": "my-editor", "args": ["--project", "{path}"]}
  }
}
```

Registered paths must be **absolute paths to individual Git repositories**
(normal or bare), not parent directories. No recursive filesystem scan runs.
SSH (`git@github.com:owner/repo.git` or `ssh://git@github.com/owner/repo.git`)
and HTTPS remotes are matched by GitHub owner/repository, case insensitively.
Registered repositories take priority over cached clones. Moved/deleted paths
are ignored; update the configuration to register their new location.

Each executable is checked on PATH before preparation starts. Arguments are
passed directly with Go's `os/exec`, without a shell. Every `{path}` is replaced
with the absolute worktree directory; if no placeholder is supplied the path is
appended as a final argument. Quote arguments using JSON strings, not shell
quoting. Each PR launches a separate process; VS Code defaults to `--new-window`.
IntelliJ's window behavior follows IDEA's project-opening preference (choose
“New window”). The editor continues running after the dashboard exits.

Repositories are reused in this order: registered repository, validated persisted
mapping/cache, new bare clone. New storage defaults to:

```text
~/.local/share/custom-visning/
├── repos/owner/repo.git/
├── worktrees/owner/repo/pr-142/
├── worktrees/owner/repo/pr-156/
├── repositories.json
└── worktrees.json
```

Linux respects an absolute `XDG_DATA_HOME`; macOS and Windows use
`os.UserConfigDir()/custom-visning` for data. These files contain paths and Git
identities, never authentication tokens. PR dashboard snapshots continue to use
the existing account-scoped JSON cache independently.

New worktrees use detached HEADs and share their repository's Git objects.
Missing commits are fetched from the base repository's `refs/pull/NUMBER/head`,
which also supports fork PRs; local refs use `refs/custom-visning/owner/repo/pr-NUMBER`.
Native Git is used because `gh pr checkout --worktree` is unavailable in many
CLI versions, and checkout behavior is less predictable with bare repositories.
Git HTTPS authentication uses the existing `gh auth git-credential` helper.
Install Git and GitHub CLI and run `gh auth login --hostname github.com` with
access to the repositories. No additional authentication mechanism is introduced.

Existing worktrees already at the dashboard's PR head open immediately without
clone or fetch. When opening an outdated worktree, the app automatically advances
it to that PR head **only if it is clean, detached, and the change is a
fast-forward**. Missing commits are fetched if necessary. This never uses reset,
force checkout, stash, merge or rebase. Attached branches, local commits, rewritten
PR history, and Git operations in progress require manual updates. Ignored files
that would be overwritten also prevent updating. Local changes are preserved.
If fetching or checkout fails, the existing worktree still opens with an update
notice; cancellation stops the operation.

`Ready`, `Modified`, and `Outdated` appear in the selected PR's detail line (and a
LOCAL column in wide terminals). Modified and Outdated can appear together.
Outdated means local HEAD differs from the latest PR head reported by the
dashboard; press **r** to refresh GitHub data and local status, then **o** to open
and attempt a safe update. Refreshing alone never changes worktree files. An
older cached snapshot cannot detect remote changes until refreshed. There is no
automatic merge/rebase or deletion command in this release.

Preparation runs asynchronously, with up to three preparation jobs and serialized
clone/fetch/worktree operations per repository. Selection is deduplicated; an
active open operation cannot be submitted twice. Preparing indicators, recent
per-PR results and errors remain visible while other navigation works. Local
status is read asynchronously at startup, refresh and preparation, never during
rendering. A failed PR does not stop the remaining selection.

### Troubleshooting worktrees

- **Editor unavailable:** install IDEA's `idea` launcher or VS Code's `code`
  command on PATH, or configure an absolute executable path. Custom commands
  must name an executable, not a shell command string.
- **Authentication/access/network failure:** check `gh auth status`, repository
  access and connectivity. Deleted PR refs may no longer be fetchable; closed
  PRs work while their refs or commits remain available.
- **Path exists / metadata conflict:** the app leaves the directory intact.
  Inspect `git -C /path/to/repo worktree list`; use Git's manual repair workflow
  for moved/stale worktrees. Do not delete directories containing user changes.
- **Interrupted preparation:** quitting cancels Git processes; temporary clone
  staging directories are removed. A partially created worktree is preserved
  and may require manual metadata repair. An editor already launched stays open.
- **Configuration/cache parse error:** fix the indicated JSON file and restart.
  Repository/worktree paths are validated before reuse. Do not edit state files
  or operate on the same worktree from multiple dashboard processes concurrently;
  serialization applies within a running application.

Development checks: `make test`, `make lint`, and `make build`. CI already runs
normal and race-enabled tests, `go vet`, formatting checks, and a build. New Git
integration tests use temporary local repositories and offline URL rewrites;
they need neither GitHub credentials nor private repository access.


### Lokal snooze

I navigasjonsmodus starter `z` snooze for valgt PR, Jira-sak eller lokal oppgave.
Skriv et positivt heltall og avslutt med `h`, `d` eller `D`; dette lagrer straks.
For eksempel betyr `z12h` tolv faktiske timer fra nå, `z3d` tre kalenderdager
med samme lokale klokkeslett og `z14D` fjorten kalenderdager fra i dag kl. 06:00.
Backspace retter tallet. Esc avbryter uten å lagre. Null, ugyldige tall og
verdier utenfor støttet tidsområde avvises. Ny snooze regnes alltid fra nå.

Listen viser antall utsatte elementer. `Z` åpner **Utsatt**, sortert etter
tidligste returtidspunkt, også for elementer utenfor gjeldende søkeresultat.
Der endrer `z` tidspunktet, `u` henter valgt element tilbake, og Esc går tilbake.
Etter lagring vises faktisk lokal returtid med tidssone og UTC-offset.

Snooze deles mellom alle faner og søk for samme tjeneste/server og URL.
En oppgaves snooze gjelder bare oppgaven; referansene beholdes og merkes hvis
referansen selv er utsatt. Oppgaver får en varig lokal ID ved første snooze.
GitHub og Jira endres aldri. Returtid sjekkes hvert sekund uten nettverkskall;
utløpte elementer er synlige også etter omstart og beholder vanlig sortering.

Kalenderberegning bruker maskinens lokale tidssone. Ved et manglende klokkeslett
under sommertidsovergangen flyttes tiden frem med gapet. Ved et klokkeslett som
forekommer to ganger velges første forekomst. Timer er alltid medgått tid.

Data lagres separat fra nettverkscache i `custom-visning/snooze.json` under
brukerens konfigurasjonsmappe (`CUSTOM_VISNING_SNOOZE` kan overstyre filstien).
Filen inneholder identitet, type, tittel, URL og returtid. Skriving bruker en
midlertidig fil med 0600-rettigheter og atomisk rename, med 0700 på nye mapper.
Lagringsfeil vises, og raden skjules først når lagringen har lykkes. En skadet
snooze-fil blokkerer nye lagringer til filen er rettet og programmet startet på nytt.
