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
h/l tabs • j/k ↑/↓ move • d details • en browser • au refresh • / filter • ? help • Ctrl+C quit
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

## Oppsettshjelp i appen

Ved oppstart sjekker appen lokalt oppsett for GitHub, Jira og Mattermost. Hvis
noe mangler, får du spørsmål om å fullføre oppsettet. Velg integrasjon med j/k
eller piltastene og trykk Enter for oppsettshjelp, eller bruk `vs` for innstillinger.
Esc fortsetter til dashboardet (lokale oppgaver hvis ingen tjeneste er klar).
Valgfrie integrasjoner kan hoppes over; forespørselen vises igjen ved neste
oppstart så lenge oppsett mangler. `--copilot` starter fortsatt AI-visningen direkte.
Sjekken gjelder lokal konfigurasjon; den bekrefter ikke tokenets tilgang på serveren.

Appen kan startes uten GitHub-innlogging. Når du åpner en visning som mangler
oppsett (`vg` GitHub, `vt` Jira, `va` AI eller `vc` Mattermost), får du hjelp
med konkrete steg for innstillinger, innlogging og nødvendig oppsett utenfor
appen. `vh` viser oppsettshjelp for aktiv side, `vs` åpner innstillingene,
og Esc går tilbake. Lange veiledninger kan rulles med j/k eller piltaster.

| Visning/funksjon | I appen | Utenfor appen |
| --- | --- | --- |
| GitHub | `vh` viser innlogging og tilgang; `vg` åpner PR-er | Installer `gh`, kjør `gh auth login`, gi repo/read:org og eventuell SSO-tilgang, eller sett `GITHUB_TOKEN` |
| Jira | Sett serveradresse i `vs`; `vt` åpner Jira | Opprett Jira PAT og sett `JIRA_TOKEN` i startterminalen |
| Mattermost-lesing | Sett server og brukernavn i `vs`; `vc`, deretter `al`/`ap`, logger inn | Konto/servertilgang; PAT må eventuelt opprettes i Mattermost |
| Mattermost-sending | `as` viser webhook-hjelp hvis oppsett mangler | Opprett incoming-webhook og sett `MATTERMOST_WEBHOOK_URL` |
| AI/Copilot | `vs` velger sesjonsmappe/innsamling; `vi` viser diagnostikk | Installer og logg inn i Copilot CLI; konfigurer eventuell OTLP-exporter |
| Editor/lokale repositories | Velg repository paths og editor i `vs` | Installer Git og editor-launcher; konfigurer Git-tilgang til private repositories |

GitHub/Jira-token og webhook leses ved oppstart; start appen på nytt fra samme
terminal etter endringer. Mattermost kan settes opp første gang uten omstart:
lagre server og brukernavn, gå tilbake og velg Enter eller `vc`. Miljøvariabler
overstyrer lagrede innstillinger. Passord, engangskoder og token skrives ikke
til konfigurasjonsfilen. Oppgaver, koblinger, utsettelse og arkiv krever ingen
ekstern innlogging.
For rask daglig oppstart fra en lokal utsjekk: kjør `make build` etter
kodeendringer og start `./bin/custom-visning` direkte. `go run` må først
kontrollere og eventuelt kompilere kildekoden før terminalvisningen starter.

## Installere release på macOS Apple Silicon

Last ned `custom-visning_Darwin_arm64.tar.gz` og `checksums.txt` fra
[siste release](https://github.com/haakonph/custom-visning-terminal/releases/latest).
Hver push til `main` publiserer automatisk en ny release etter beståtte sjekker.
Fra mappen med de nedlastede filene:

```bash
shasum -a 256 -c checksums.txt
tar -xzf custom-visning_Darwin_arm64.tar.gz
# Hvis macOS har satt quarantine på den verifiserte binæren:
xattr -d com.apple.quarantine custom-visning
mkdir -p "$HOME/.local/bin"
install -m 755 custom-visning "$HOME/.local/bin/custom-visning"
export PATH="$HOME/.local/bin:$PATH"
gh auth login --hostname github.com
custom-visning
```

Legg PATH-linjen i `~/.zshrc` for senere økter. Første versjon distribuerer kun
macOS arm64. Binæren er ikke Apple-signert eller notarized; følg macOS sin
vanlige godkjenning for nedlastet programvare ved behov. `xattr -d` over
fjerner quarantine-attributtet fra den utpakkede binæren etter kontroll av
sjekksummen. Hvis attributtet mangler, kan macOS vise «No such xattr»;
da kan du fortsette installasjonen.

## Sjekke og laste ned oppdateringer

```bash
custom-visning --check-update
custom-visning --update
```

`--check-update` viser installert versjon og siste stabile release. `--update`
spør før den laster ned, verifiserer og erstatter den kjørende binæren,
uavhengig av hvilken mappe kommandoen startes fra. Bare macOS arm64 har
publiserte releasearkiver foreløpig. Midlertidige filer fjernes etterpå, også
ved feil. Kommandoene starter ikke dashboardet.

Oppdateringer bruker et eget token fra **`CUSTOM_VISNING_UPDATE_TOKEN`**.
Sett det i miljøet via din vanlige lokale hemmelighetshåndtering. Tokenet trenger
bare lesetilgang til releaser i `haakonph/custom-visning-terminal`: for et
fine-grained PAT velger du repositoryet og **Contents: read**. Ingen
skriverettigheter eller organisasjonsrettigheter er nødvendig for oppdateringer.
Tokenet lagres ikke av programmet og skal ikke legges i kildekode eller config.

Hvis det egne tokenet mangler eller er tomt, brukes `GITHUB_TOKEN`, deretter
`gh auth token --hostname github.com` som reserve. Et eksplisitt token som blir
avvist utløser ingen overgang til en annen identitet. Dashboardet fortsetter å
bruke sin eksisterende autentisering, uavhengig av oppdateringstokenet.
Tokenet sendes bare til GitHub API, og fjernes ved videresending til
nedlastingslagring. API-feilbodyer og tokenverdier vises ikke.

Versjonene sammenlignes som identiteter: prosjektet publiserer både tagger og
`main-<run>.<attempt>-<sha>`, mens branchbygg heter `build-<sha>`.
Ved forskjell vises begge versjonene, slik at du selv kan vurdere om du vil
laste ned siste stabile release. Dette kan også være en eldre release enn et
lokalt bygg. Sjekk og nedlasting skjer bare når du kjører kommandoene over.

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

Alle snarveier følger nå ett mønster. Se [fullstendig tastaturoversikt](docs/keyboard.md),
inkludert tastene som ikke var definert i den opprinnelige listen.

- `j/k` eller ↑/↓ velger rad; `h/l` eller ←/→ bytter fane.
- `d` viser detaljer, `/` søker, `?` viser hjelpen.
- Space og Shift+↑/↓ velger flere; `x` fjerner alle markeringer.
- `r` relatert, `v` visning, `t` koble, `e` åpne, `a` handling.
- `o/g/t/a/c` velger oppgaver/GitHub/Jira/AI/chat under en starttast.
- `atfq` flytter tilhørende Jira-saker til QA direkte fra GitHub.
- `au` oppdaterer, `vs` åpner innstillinger og Ctrl+C avslutter.

Gamle enkelttaster, talltaster for faner og Tab-snarveier er fjernet. Enter brukes
bare til å bekrefte dialoger/tekstfelt. AI og chat har også flervalg, koblinger,
relasjoner, snooze og lokalt arkiv. Typene kan inngå i kjedede handlinger.

GitHub-fanene er egne åpne PR-er uten drafts, egne drafts, direkte reviewforespørsler
og teamforespørsler. Filteret beholdes ved fanebytte, og oppdatering skjer asynkront.
Delvise resultater beholder cache og tidspunkt for siste fulle synk. UNKNOWN betyr
at GitHub ikke har gitt en kjent status; det er ikke bevis på at sjekker har bestått.

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

Hver autentisering har en egen SQLite-database, navngitt med et SHA-256-fingeravtrykk.
Selve tokenet lagres ikke. Filen inneholder verifisert brukernavn, PR-data og siste
fullførte synkronisering per visning. Tokenbytte gir ny cache. Første oppstart
krever nettverk for identiteten. Senere oppstarter viser cache umiddelbart, også
uten nettverk, mens nye data hentes i bakgrunnen. Manglende autentisering krever
fortsatt innlogging. Korrupte/ukjente cacheversjoner ignoreres med en melding.

Cachemapper opprettes med `0700`, filer med `0600`. Cachen inneholder privat
PR-metadata i klartekst: slett cachemappen for å fjerne lokale data. Skriving skjer
i atomiske SQLite-transaksjoner. Skriving er
serialisert og samles opp for å unngå at eldre svar overskriver nyere snapshots.

REST Search API brukes til oppdagelse med alle sider frem til GitHubs grense på
**1 000 treff per søk**, også per team. Søket bruker det verifiserte brukernavnet
istedenfor `@me`, `sort=updated` og `order=desc`. Teammedlemskap pagineres på tvers
av organisasjoner. Direkte forespørsler bruker `user-review-requested:LOGIN`;
team bruker `team-review-requested:ORG/SLUG -author:LOGIN`. Egne PR-er skjules
også fra tidligere cache i Team Review Requests.

GitHub-tabellen har egen Jira-kolonne med nøkler fra PR-tittel, beskrivelse og
branchnavn. Nøklene fjernes fra viste PR-titler, inkludert tomme `[]`, `{}` og
`()`. Originaltekst og lenker beholdes for søk og handlinger. Fra 100 kolonner
vises `AUTO` (`ON`/`OFF`/`UNKNOWN`) og `INFO`. `INFO` viser `CONFLICT` ved
mergekonflikter, ellers køplass og køstatus (tomt felt når PR-en ikke er i kø).
Konflikter prioriteres i kolonnen; full køstatus beholdes under tabellen og i
detaljvisningen, som også forklarer eventuelle mergekonflikter.
Statusene finnes også under tabellen og i detaljvisningen på smalere terminaler.

Alle fire PR-fanene grupperer PR-er under Jira-saken, på tvers av repoer.
Gruppene viser Jira-nøkkel, sakstittel når den er lastet fra Jira, og antall PR-er.
Første nøkkel i tittel, beskrivelse eller branchnavn bestemmer gruppen; hver PR
vises én gang, og alle Jira-koblinger beholdes. Relaterte PR-er grupperes under
saken du kom fra. PR-er uten nøkkel samles under **Uten Jira-sak** til slutt.
I **My Review Requests** og **Team Review Requests** samles alle PR-er fra
Dependabot i en egen **Dependabot**-gruppe helt nederst, også når de har Jira-nøkkel.
Gruppene følger rekkefølgen til første PR, med eksisterende sortering innad.
Søk og filtre brukes før gruppering. Gruppeoverskrifter hoppes over ved
navigasjon og markering; handlinger gjelder fortsatt de valgte PR-ene.

Organisasjonsprefikset `skatteetaten-ghe/` skjules i viste repository- og teamnavn,
og i GitHub-referanser i oppgaver, arkiv og relaterte visninger. Velg
**GitHub · skjulte organisasjonsprefikser** i `vs` for å endre listen; `[]` viser
fulle organisasjonsnavn. Innstillingen `github.hidden_prefixes` endrer bare presentasjonen.
Prefikset `glsk-` skjules også i reponavn, for eksempel blir
`skatteetaten-ghe/glsk-service` vist som `service`. Dette gjelder også
GitHub-referanser og AI-visningen. Teamnavn, URL-er og lagrede repoidentiteter beholdes.

GraphQL `nodes(ids:)` beriker opptil 40 PR-er per kall med draft-status,
reviewDecision, requested reviewers/teams, siste commits statusCheckRollup og
mergeable, automerge og mergekø (status og plass). Ingen ubetingede HTTP-kall per PR. Opptil 100 reviewer-forespørsler per
PR hentes; større lister markeres delvise. Ved GraphQL-/tilgangsfeil beholdes
søkeresultatene med eksplisitt ukjente statuser og advarsel. Resultater sorteres
lokalt etter sist oppdatert og dedupliseres på URL.

I GitHub-visningen slår `a+` på automerge (merge when ready), og `a-` slår det
av. Handlingen gjelder markerte PR-er, ellers aktiv PR, og virker også i
detaljvisningen. `AUTO` viser `ON`, `OFF` eller `UNKNOWN`; statusen vises også
under listen og i detaljene. `QUEUE` viser køstatus og plass når GitHub har
returnert dette. Bruk `au` for å hente fersk status.

GitHub håndhever tilganger, nødvendige sjekker, reviews og branch-regler.
Aktivering krever en åpen PR som ikke er draft og at repository tillater
automerge. Appen velger en tillatt merge-metode i rekkefølgen merge commit,
squash, rebase. GitHub kan merge når kravene er oppfylt. Statusen fra GitHub
oppdateres i alle lastede faner og lagres i cachen. Ved feil i flervalg stoppes
resten; allerede oppdaterte PR-er beholdes og antallet vises i feilmeldingen.

Review-status bruker GitHubs `reviewDecision` når den finnes. For PR-er uten
samlet status, for eksempel stablede PR-er mot brancher uten review-regler,
brukes `latestOpinionatedReviews`: siste gjeldende vurdering per reviewer.
`CHANGES` har forrang over `APPROVED`; uten vurderinger vises `PENDING` ved
utestående forespørsler, ellers `NO REVIEW`. Kommentarer erstatter ikke en
vurdering, og tilbaketrukne reviews teller ikke. Godkjenninger arves ikke fra
andre PR-er i stacken og betyr ikke at alle mergekrav er oppfylt.

Under tabellen og i detaljvisningen vises antall godkjenninger, endringsønsker
og utestående forespørsler. Teamforespørsler telles separat. Ny review-forespørsel
til en som allerede har godkjent telles som ventende i tillegg til vurderingen.
Status fra individuelle vurderinger merkes `(vurderinger)`, mens GitHubs samlede
status merkes `(GitHub)`. Review-opptellingen lagres i cachen. Opptil 100 reviewere
og 100 forespørsler leses per PR; manglende eller avkortede data markeres som
delvis opptelling, og fallback-statusen forblir `UNKNOWN`.

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

CI-kolonnen viser `FAIL` når en påkrevd sjekk feiler, og `WARNING` når bare valgfrie sjekker feiler. Feil i sjekker med `tekst-sync` i navnet vises som `TEKSTER`, også når sjekken er påkrevd. Ved flere feil prioriteres `FAIL`, deretter `TEKSTER` og `WARNING`. `FAIL` vises i rødt, `TEKSTER` i lilla og `WARNING` i gult. Sjekker som kjører, vises fortsatt som `PENDING`; vellykkede sjekker gir `PASS`.

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
taggen som versjon. Versjonen bygges inn i `--version` og vises øverst på
hjelpsiden (`?`). Publisering bruker workflowens `GITHUB_TOKEN`; bare publiseringsjobben har `contents: write`.
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
export JIRA_PATTERN='KAKE-[0-9]{3,5}'  # valgfritt mønster for Jira-nøkler i PR-er
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

**Trykk `vg` for GitHub og `vt` for Jira.** Hurtigtastene fungerer
i navigasjonsmodus. Mens du redigerer et filter eller JQL, skrives bokstavene
som vanlig tekst; avslutt redigeringen før du bytter. Begge visninger beholder
valgt fane, filter, markert rad og data. Byttet utløser ingen ny henting etter
første åpning; bruk `au` når du vil oppdatere. Snarveiene vises øverst i begge visninger.
GitHub-feil og Jira-feil håndteres hver for seg under kjøring.

Standardfanene er **Mine**, **Prosjekt**, **Uten ansvarlig** og **Aktiv sprint**.
Alle bruker `statusCategory != Done`. Prosjektfanen viser alle tilgjengelige
prosjekter når prosjektfilter ikke er satt. Aktiv sprint krever Jira Software.

| Tast | Jira-handling |
| --- | --- |
| `vg` | Gå til GitHub; behold tilstanden |
| `vt` | Gå til Jira; behold tilstanden |
| `h` / `l`, `←` / `→` | Bytt mellom standardfaner og lagrede søk |
| `j` / `k`, `↓` / `↑` | Velg sak |
| `en` | Åpne valgt sak i Jira |
| `/` | Filtrer lokalt blant hentede saker |
| `vf` | Rediger kombinerbare hurtigfiltre i standardfaner |
| `vj` | Rediger og kjør full JQL, inkludert ønsket `ORDER BY` |
| `aq` | Lagre gjeldende JQL med navn; samme navn erstatter eksisterende søk |
| `x` | Fjern markeringer; søk og filtre beholdes |
| `au` | Hent aktivt søk på nytt |
| `Esc` | Avbryt redigering eller fjern lokalt tekstfilter |
| `?` | Vis/skjul hjelp |
| `Ctrl+C` | Avslutt og kanseller nettverksarbeid |

Bruk h/l eller venstre-/høyrepil for å velge fane. Lokalt tekstfilter beholdes ved fanebytte. Flere enn fem lagrede
søk nås med h/l eller venstre-/høyrepil.

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

`JIRA_PATTERN` er et Go-regulært uttrykk som brukes til å finne Jira-nøkler i
PR-tittel, beskrivelse og branchnavn, både i detaljvisningen og ved navigasjon
mellom relaterte PR-er og Jira-saker. Eksemplet over godtar `KAKE-123` til
`KAKE-12345`, men ikke 2 eller 6 sifre, andre prefikser eller treff inni ord.
Trykk `,` for å åpne **Innstillinger** fra GitHub, Jira eller Copilot.
`vs` fungerer fra alle visninger. Velg felt med ↑/↓ eller j/k og trykk
Enter for å redigere. Enter validerer og lagrer feltet; Esc avbryter redigering,
og Esc på innstillingssiden går tilbake til forrige visning.

Alle appinnstillinger ligger i én fil:
`os.UserConfigDir()/custom-visning/config.json`. Bruk `CUSTOM_VISNING_CONFIG`
for å velge en annen fil. Siden viser filstien og felter for Jira-server,
standardprosjekt, Jira-mønster, lagrede søk, lokale repoer, editorer og
Copilot-innstillinger (mapper, port, innsamling, innholdsvalg, Jira-mønster og
IntelliJ-launcher). Lister og editorargumenter redigeres som JSON, for eksempel
`["/sti/til/repo"]` og `["--new-window", "{path}"]`.

Jira-mønsteret og GitHub-prefiksene tas i bruk med en gang. Tom verdi gjenoppretter
`[A-Z][A-Z0-9_]*-[0-9]+`. Øvrige innstillinger tas i bruk etter omstart.
Miljøvariabler overstyrer filverdiene ved oppstart, så fjern eksempelvis
`JIRA_PATTERN` for å bruke det lagrede mønsteret. Trykk `?` i innstillingssiden for rullbar hjelp til GitHub-innlogging, Jira-PAT
og Mattermost-webhook, inkludert trygg innlesing i Bash. Jira-serverfeltet viser
eksempeladressen `https://jira.example.no/jira`. Token settes fortsatt via
miljøet og lagres ikke av innstillingssiden. Oppgaver, snooze-data, sesjonshistorikk
og cacher er datafiler og holdes adskilt fra innstillingene.

Eksisterende `repository_paths` og `editors` i config.json beholdes.
Ved oppstart importeres gammel `jira.json` (eventuelt filen pekt på av
`JIRA_CONFIG`) automatisk hvis den felles filen ennå ikke har en `jira`-seksjon.
Originalfilen beholdes. Nye endringer skrives bare til den felles filen.

Eksempel på deler av den felles filen (utelatte felt bruker standardverdier):

```json
{
  "github": {"hidden_prefixes": ["skatteetaten-ghe"]},
  "repository_paths": ["/sti/til/repo"],
  "editors": {"default": "vscode"},
  "jira": {
    "base_url": "https://jira.example.no",
    "project": "KAKE",
    "pattern": "KAKE-[0-9]{3,5}",
    "queries": [{"name": "Mine feil", "jql": "assignee = currentUser() AND issuetype = Bug"}]
  },
  "copilot": {
    "port": 4318,
    "enabled": true,
    "capture_content": true,
    "show_content": true,
    "jira_pattern": "KAKE-[0-9]{3,5}"
  }
}
```

Søkeresultater caches under appens cachemappe i undermappen `jira`, adskilt per
server, token og full JQL med SHA-256-fingeravtrykk. Cachen inneholder sakens
metadata og beskrivelse i klartekst, med mapper `0700` og filer `0600`. Slett
undermappen for å fjerne lokale Jira-data. Ved oppstart/søk vises eventuell cache
mens data hentes. Feil beholder forrige resultat og flytter ikke siste vellykkede
synkronisering. Alle sider må lykkes før et nytt resultat erstatter cachen.
Jiras `ORDER BY` beholdes. Oppdateringer skjer ved oppstart, søk/fanebytte og `au`.
Nye søk kansellerer forrige søk, og forsinkede svar ignoreres.

Klienten har 30 sekunders request-timeout og to minutter per søk. Ved ugyldig JQL,
401/403, rate limit eller serverfeil vises en melding; rå feilbodyer og token
vises ikke. Redirects følges ikke, og nettleserlenker begrenses til konfigurert
Jira-server og context path. Integrasjonen endrer ingen Jira-saker.


## Navigering mellom GitHub og Jira

Trykk `rt` fra GitHub for relaterte Jira-saker, eller `rg` fra Jira for relaterte PR-er. `ro` finner den første interne oppgaven som inneholder valgt referanse, og `rc` viser Mattermost-omtaler av aktiv PR.

Når flere treff vises, bruker valglisten en tabell med type, nøkkel, bruker og
tittel. Bruker er PR-forfatteren i GitHub, ansvarlig i Jira og meldingsforfatteren
i Mattermost. Manglende bruker vises som `—`. I smale terminalvinduer prioriteres
bruker og tittel. Søk med `/`, marker med Space og gå til valgt treff med Enter.

- Fra GitHub: Jira-nøkler (for eksempel `TEAM-123`) hentes fra PR-tittel, beskrivelse og branchnavn. Jira viser alle refererte saker i et eget JQL-søk, uavhengig av hurtigfiltrene.
- Fra Jira: viser relaterte PR-er fra alle fire lastede GitHub-visninger, uten duplikater. Dette er ikke et søk i alle organisasjonens PR-er.
- `en` åpner valgt PR eller sak i nettleseren. `Esc` i GitHub fjerner avgrensningen til en Jira-sak. I Jira velger du en vanlig fane for å forlate relasjonssøket.
- `vg` og `vt` bytter visning. Begge integrasjonene må være konfigurert for navigering begge veier.

Nøkler må bruke store bokstaver, og koblingen krever ikke skrivetilgang til GitHub eller Jira. Oppdater GitHub-data med `au` for å hente beskrivelser og branchnavn som mangler i gammel cache.


## Oppgaver (fleksibelt konsept)

En oppgave er en lokal samling av eksplisitte referanser. Den kan være en tynn
wrapper rundt én Jira-sak eller PR, inkludert en PR fra review-fanene, eller samle
flere Jira-saker og PR-er på tvers av repoer. Den har egen tittel og trenger ingen
Jira-nøkkel. Endringer i oppgaven endrer ikke Jira eller GitHub.

- `an` fra valgte elementer lager en oppgave fra valgt rad og åpner oppgavesiden.
- `vo` åpner oppgavesiden; `j/k` velger oppgave i listen; `h` / `l` blar mellom oppgavene.
  På brede terminaler vises oppgavelisten til venstre og detaljene til høyre.
  På smale terminaler bytter du mellom liste og detaljer.
- `/` søker i oppgavetitler og tilknyttede referanser. `Enter` beholder søket,
  `Esc` nullstiller det.
- `d` går fra listen til referansene. `Esc` går tilbake
  til listen. I detaljene velger `j/k` referanse og `en` åpner den.
- `vg` / `vt` går til GitHub/Jira. Finn en rad og trykk `to`, velg en oppgave og bekreft med Enter for å legge den til. Samme referanse legges ikke til to ganger.
- `an` lager en tom oppgave, `ae` redigerer tittelen, `Enter` lagrer og `Esc` avbryter
  tittelredigeringen. En tom oppgave kan fylles ut senere.
- `d` viser detaljer. `ad` fjerner valgt kobling i detaljene, eller sletter oppgaven i listen.
- `ak` arkiverer/gjenoppretter lokal oppgave. `vb` viser arkivet; `ak` gjenoppretter derfra.
- `vg` / `vt` / `va` / `vc` går til ønsket visning. `Esc` går først til listen,
  deretter nullstilles et eventuelt søk, og til slutt tilbake til provider-visningen.

Oppgavene lagres atomisk i `custom-visning/tasks.sqlite` under operativsystemets
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
historikken i bakgrunnen ved oppstart og leser nye data hvert andre sekund.
Terminalvisningen venter ikke på at Copilot-lageret skal leses. Copilot viser
lasting til historikken er klar; en lesefeil vises i Copilot uten å overskrive
lageret eller hindre GitHub-/Jira-visningen i å starte. Samtale,
verktøyargumenter/resultater, feil, klient, modell, tokens og prosjekt vises når
kilden inneholder dem. Det lokale Copilot-filformatet er ikke en stabil offentlig
API; ukjente hendelser beholdes og ugyldige poster rapporteres i diagnostikk.
Kildemappene og filene endres aldri; symlinker følges ikke.

Terminalen har et eget datalager i `os.UserConfigDir()/custom-visning-terminal/copilot.sqlite`
(på macOS normalt `~/Library/Application Support/custom-visning-terminal/copilot.sqlite`,
på Linux normalt `~/.config/custom-visning-terminal/copilot.sqlite`). Lagrede sesjoner,
inkludert mottatt telemetri, beholdes mellom oppstarter, også om kildefilene fjernes.
Databasen oppdateres atomisk med tilgang `0600`, og nye lagringsmapper opprettes med `0700`.
Lageret er separat fra både GitHub-cachen og den andre appens database.

Hvis appen melder `could not read Copilot store`, viser feilmeldingen hvilken
fil som ikke kan leses. Filen beholdes urørt. Kontroller filsti og lesetilgang;
en skadet SQLite-database eller ugyldig/ufullstendig JSON ved import må
gjenopprettes fra en sikkerhetskopi. For å
starte med et separat lager uten å endre originalen, sett `COPILOT_STORE_PATH`
til en ny, absolutt filsti. Lokal sesjonshistorikk importeres på nytt når
innsamling er aktivert; telemetri som bare finnes i det gamle lageret følger
ikke med. Gyldige lagre over 128 MiB støttes også.

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
| `COPILOT_STORE_PATH` | Se datalagerstien ovenfor | Absolutt sti til terminalens eget SQLite-lager (eldre JSON-stier støttes) |
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
- **eee** åpner den valgte sesjonens arbeidsmappe i IntelliJ fra listen eller detaljene.
- **eew** åpner sesjonens Git-/worktreerot i IntelliJ.
- **j/k** eller pil opp/ned velger sesjon eller ruller i detaljene.
- **vi** viser diagnostikk; **Esc** går tilbake og fjerner søket.
- **au** oppdaterer manuelt. Visningen oppdateres også hvert tredje sekund.
- **g** og **J** bytter til GitHub/Jira når de er konfigurert.

### Åpne IntelliJ fra en sesjon

Trykk **eee** i Copilot-visningen for chattens arbeidsmappe (`cwd`), eller **eew**
for Git-/worktreeroten (`gitRoot`). **ee** venter på tredje tast; **Esc** avbryter.
Hvis den foretrukne mappen mangler eller ikke finnes lokalt, brukes prosjektroten
og deretter den andre mappen. Hvis chatten jobber i worktreeroten, åpner begge
samme mappe. I andre visninger og relasjonskjeder beholdes editorhandlingen som før.
IntelliJ åpner prosjektet eller
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
Press **ee** to open the selection in IntelliJ, or the highlighted PR when
nothing is selected. **ev** opens an editor chooser using j/k and Enter;
Esc cancels. **en** opens the PR in your browser.

In the GitHub view, **aw** deletes the selected PRs' local worktrees (or the
highlighted PR if nothing is selected). Review the confirmation and press **y**;
**n** or **Esc** cancels. Local changes, untracked files, locks and ongoing Git
operations prevent removal. Ignored files in the checkout are removed. Branches
and commits remain. Press **ee** afterwards to create a fresh worktree.

The shared app configuration (also editable with `,` → Innstillinger) is stored at
`os.UserConfigDir()/custom-visning/config.json` (normally
`~/.config/custom-visning/config.json` on Linux or
`~/Library/Application Support/custom-visning/config.json` on macOS).
Edit it and restart the application:

```json
{
  "repository_paths": ["/home/alice/src/frontend", "/home/alice/src/backend"],
  "workspace": {
    "warm": true,
    "install": true,
    "main_branch": "main",
    "maven_flags": ["-Dmaven.gitcommitid.skip=true"],
    "indexer": {
      "command": "",
      "args": ["warmup", "{path}"]
    }
  },
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
├── main/owner/repo/
├── worktrees/owner/repo/pr-142-fix-login/
├── worktrees/owner/repo/pr-156-add-search/
├── logs/
├── repositories.sqlite
└── worktrees.sqlite
```

Linux respects an absolute `XDG_DATA_HOME`; macOS and Windows use
`os.UserConfigDir()/custom-visning` for data. These files contain paths and Git
identities, never authentication tokens. PR dashboard snapshots continue to use
the existing account-scoped SQLite cache independently.

On startup and GitHub refresh, repositories from all loaded PR tabs are queued
for background preparation. A background timer also keeps these main checkouts
updated without a manual refresh. One main checkout per repository is fetched and
fast-forwarded, at most once per five minutes while the app is running. Existing
PR checkouts are updated only when opened. Set `workspace.warm` to false to
disable background preparation, or change `main_branch` for repos using master.
Registered working directories are never checked out or pulled: a separate
main worktree shares their objects. If main is already checked out, that separate
checkout uses `custom-visning/warm-main`. Local changes, switched branches and
Git operations in progress prevent updating the main checkout.

New PR worktrees use the PR's actual branch name when available. A branch already
in use gets an independent `custom-visning/pr-NUMBER` branch; existing branches
and local commits are preserved. New directories include a sanitized PR title;
the PR number is the stable identity, so renaming a PR reuses its old directory.
Legacy paths are reused, and clean detached checkouts are attached to a branch
on their next opening. Dirty detached checkouts remain untouched.
All worktrees share their repository's Git objects.
Missing commits are fetched from the base repository's `refs/pull/NUMBER/head`,
which also supports fork PRs; local refs use `refs/custom-visning/owner/repo/pr-NUMBER`.
Native Git is used because `gh pr checkout --worktree` is unavailable in many
CLI versions, and checkout behavior is less predictable with bare repositories.
Git HTTPS authentication uses the existing `gh auth git-credential` helper.
Install Git and GitHub CLI and run `gh auth login --hostname github.com` with
access to the repositories. No additional authentication mechanism is introduced.

Existing worktrees open after a quick local check of their repository identity
and worktree root. The editor launches before waiting for repository locks,
scanning local changes, fetching commits, or preparing the Maven project. These
tasks continue in the background with `Opened · Updating` shown in the terminal.
The next selected worktree can open while previous ones are still updating.
Worktrees already at the dashboard's PR head need no clone or fetch.
When opening an outdated worktree, the app automatically advances it to that PR
head **only if it is clean, on its managed PR branch, and the change is a
fast-forward**. Missing commits are fetched if necessary. This never uses reset,
force checkout, stash, merge commits or rebase. Other branches, local commits,
rewritten PR history, and Git operations in progress require manual updates. Ignored files
that would be overwritten also prevent updating. Local changes are preserved.
If fetching or checkout fails, the editor remains open and an update notice is
shown; cancellation stops the background operation.

`Ready`, `Modified`, and `Outdated` appear in the selected PR's detail line (and a
LOCAL column in wide terminals). Modified and Outdated can appear together.
Outdated means local HEAD differs from the latest PR head reported by the
dashboard; refresh GitHub data, then press **ee** to open and attempt a safe update.
Refreshing can update the main checkout but never changes PR worktree files. An
older cached snapshot cannot detect remote changes until refreshed. There is no
automatic merge/rebase or deletion command in this release.

The app automatically removes its managed PR worktrees from IntelliJ IDEA's
recent-project list once GitHub confirms that the PR is closed or merged.
It checks at startup, on GitHub refresh and every five minutes while running,
including when `workspace.warm` is disabled. Old worktrees tracked by the app
are included even when the PR is no longer in any dashboard tab. Disappearing
from a review queue, incomplete searches, API errors and inaccessible PRs do
not trigger removal. Worktree files, local edits and Git branches are retained.
Standard IntelliJ IDEA/Community settings directories on Linux, macOS and
Windows are supported, including legacy settings directories. Custom IDEA
configuration directories are not discovered. IntelliJ may keep its recent
projects in memory while running; changes may only appear after restarting
the IDE, and later app checks retry if IntelliJ writes an old entry back.

Opening runs asynchronously, with up to three opening jobs and serialized
clone/fetch/worktree operations per repository. Selection is deduplicated; an
active open operation cannot be submitted twice. Preparing indicators, recent
per-PR results and errors remain visible while other navigation works. Local
status is read asynchronously at startup, refresh and preparation, never during
rendering. A failed PR does not stop the remaining selection.

### Dependencies and headless IntelliJ

Main checkouts and clean, current PR worktrees are prepared using their own
root build files. Maven uses `./mvnw -B -DskipTests -Dmaven.gitcommitid.skip=true install` (or `mvn`), Gradle
uses `./gradlew --no-daemon assemble -x test` (or `gradle`), and npm uses `npm ci`
when a lockfile exists, otherwise `npm install`. Java builds prefer Maven when
both Maven and Gradle root files exist; npm also runs for mixed projects.
This executes the repository's build/install scripts. Set `workspace.install`
to false to disable it. Java/Node and any required SDK versions must already be
available in the app's environment. Monorepos should expose their build from the
repository root. Download caches are shared through the tools' normal user
caches; build output and node_modules stay separate per checkout.

Configure Maven flags in `vs` under **Lokalt · Maven-flagg (JSON)** or using
`workspace.maven_flags`, for example
`["-Dmaven.gitcommitid.skip=true", "-U", "-Pdev"]`. This list replaces the default
flags; use `[]` to remove them. Each entry is passed as a separate argument,
without shell interpretation, before `install`. Existing configurations inherit
the default when the field is absent. Flags apply to both main and PR preparation;
changing them invalidates the preparation cache on the next eligible run.

Headless indexing is enabled by setting **IntelliJ headless · kommando** in `vs`
to a compatible launcher. For an IntelliJ installation with the Remote Development
warmup command, configure its `bin/remote-dev-server.sh` and leave the arguments
as `["warmup", "{path}"]`. For another version or platform, supply an executable
wrapper and its argument list that imports/indexes the project and exits when
finished. An empty command disables indexing and is reported in the UI. The
normal `idea` GUI launcher is configured separately under Editor settings.

Preparation runs once per checkout, commit and workspace configuration after a
successful run. Failures are retried on a later opening/background cycle. Build and
indexer output goes to private files under `logs/`; errors show the log path.
Failed preparation still opens the PR with a notice so setup can be repaired in
the IDE. Quitting cancels active preparation. Main is limited to 15 minutes per
repository and PR opening to 20 minutes including any repository-lock wait.
Indexing main does not guarantee reusable indexes at a new worktree path, so the
headless command also runs for each clean, current PR checkout before opening.
IDE detection of the PR depends on its GitHub integration; a fallback branch has
no automatic upstream/push target, especially for fork PRs.

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
- **Configuration/cache parse error:** fix the indicated configuration JSON file, or restore the affected SQLite database from backup, and restart.
  Repository/worktree paths are validated before reuse. Do not edit state files
  or operate on the same worktree from multiple dashboard processes concurrently;
  serialization applies within a running application.

Development checks: `make test`, `make lint`, and `make build`. CI already runs
normal and race-enabled tests, `go vet`, formatting checks, and a build. New Git
integration tests use temporary local repositories and offline URL rewrites;
they need neither GitHub credentials nor private repository access.

### Strukturert SQLite-lagring med sqlc

GitHub, Jira, Copilot og lokale brukerdata lagres i egne, strukturerte tabeller.
Dagens visninger og config-lagring er uendret. SQL-filer er kilden til skjema og
spørringer; sqlc **v1.30.0** genererer de typede Go-modellene og spørringsmetodene.
Generert kode er sjekket inn, så vanlig bygging krever verken sqlc eller CGO.

```text
internal/storage/
├── migrations/  # nummererte, innebygde SQL-migreringer
├── queries/     # SQL-spørringer med sqlc-navn
├── dbgen/       # generert Go-kode; skal ikke redigeres manuelt
├── github.go    # mapping mellom eksisterende modeller og tabeller
├── jira.go
├── copilot.go
└── local.go
sqlc.yaml        # generatoroppsett, separat fra appens config
```

Installer sqlc v1.30.0 fra https://sqlc.dev/ og kjør `make generate` etter endringer
i SQL-filene. `make check-generated` og CI kontrollerer at generert kode følger
SQL-kildene. Nye skjemaendringer legges i neste nummererte migrering; migreringer
som er tatt i bruk, endres ikke. `schema_migrations` registrerer utførte versjoner.
En database med nyere skjema enn programmet støtter, avvises uten dataskriving.

#### Automatisk skjemadokumentasjon

Kjør `npm run db:docs` etter en migreringsendring og commit både
[`docs/database/schema.md`](docs/database/schema.md) og
[`docs/database/schema.sql`](docs/database/schema.sql). Kommandoen krever Node/npm
og Go-versjonen fra `go.mod`; `npm install`, SQLite CLI og nye npm-pakker trengs
ikke. Den kjører den samme migreringsmotoren som appen, mot en isolert SQLite-database
i minnet. Brukerdata og lokale databasefiler berøres ikke.

`schema.md` inneholder et Mermaid ER-diagram med primær- og fremmednøkler.
`schema.sql` inneholder hele det resulterende skjemaet, inkludert views, eksplisitte
indekser, triggere og migreringstabellen, uten data eller SQLite-interne objekter.
SQL-filen er fasiten for constraints og kardinalitet; diagrammets relasjoner er
konservative. Genereringen har stabil sortering og ingen tidsstempler.

- `npm run db:docs:check` kontrollerer filene uten å endre dem og feiler også ved manglende filer.
- `go test ./cmd/db-docs ./internal/storage` tester migrering, determinisme,
  SQL-gjenoppretting, fremmednøkler og kontroll av utdatert dokumentasjon.
- `node --test scripts/schema-report.test.mjs` tester PR-rapport og størrelsesgrenser.

Workflowen `Database documentation` kjører på relevante PR-endringer, inkludert
migreringer, og på `main`. Ved PR sammenlignes det genererte SQL-skjemaet med
migreringene på gjeldende `main`, ikke med dokumentasjonen som ligger der.
Revisjoner før SQLite ble innført får et eksplisitt tomt sammenligningsgrunnlag.
Manglende eller ødelagte migreringer etter innføringen skal gi feil.
CI laster opp skjemadiff, Mermaid-diagram og SQL-filer som artifact før kontrollen
av inncommittet dokumentasjon. Kjør kommandoen lokalt og commit endringene selv;
GitHub Actions committer aldri.

Workflowen `Database documentation comment` oppdaterer én kommentar per PR,
også når dokumentasjonskontrollen feiler. Lange differ avkortes i kommentaren;
SQL-filene i artifactet gir hele grunnlaget. Kommentering bruker `workflow_run`
og kjører bare kode fra standardgrenen, uten utsjekking eller kjøring av PR-kode.
Denne workflowen må derfor være flettet inn i `main` før automatisk kommentering
blir aktiv, også for fork-PR-er. Genereringsjobben har bare `contents: read`;
kommentarjobben har bare `actions: read` og `pull-requests: write` og kontrollerer
at rapporten fortsatt gjelder PR-ens siste commit. Repoets Actions-innstillinger
må tillate disse rettighetene.

| Område | Tabeller |
| --- | --- |
| GitHub | `accounts`, `github_repositories`, `github_pull_requests`, `github_labels`, `github_pull_request_reviewers`, `sync_state`, `github_view_memberships` |
| Jira | `jira_projects`, `jira_issues`, `jira_labels`, `jira_queries`, `jira_query_memberships` |
| Copilot | `copilot_sessions`, `copilot_events`, `copilot_traces`, `copilot_usage` |
| Lokalt | `tasks`, `task_references`, `snoozes`, `local_repositories`, `local_worktrees` |

PR-er lagres én gang i hvert kontolager og knyttes til faner med egne rader som
bevarer rekkefølgen. Jira beholder JQL-rekkefølgen på samme måte. Copilot har
kolonner for sammendraget som dagens oversikt beregner, og egne hendelses- og
forbruksrader. Fleksibel Copilot-metadata og rå hendelsesdata kan fortsatt være
JSON; hele domenedatasett lagres ikke lenger som JSON-dokumenter. SQL-viewen
`overview_items` gir en felles projeksjon for databaseinspeksjon; appen får ingen
ny felles visning.

Eksisterende lagringsgrenser beholdes: GitHub-cache er isolert etter konto/token,
Jira-cache etter server/token/spørring, mens lokale brukerdata er separate fra
cache. Tabellene ligger derfor i eksisterende SQLite-filer, ikke i én ny felles
fil. Dette bevarer også eksisterende miljøoverstyringer og muligheten til å
slette cache uten å slette oppgaver, snooze og Copilot-historikk. Fremmednøkler er
aktivert, og en oppdatering av et datasett og relasjonene skjer atomisk.
Oppgavereferanser beholder provider, URL og visningssnapshot uten harde
fremmednøkler til provider-cache, slik at de overlever tømming av cache.

Gamle JSON-filer leses og valideres før første import, og beholdes uendret som
sikkerhetskopi. SQLite-filer fra den første lagringsversjonen med `documents`
importeres også: dokumentet fjernes først når de strukturerte radene er committet.
Ved konverteringsfeil rulles hele dataimporten tilbake. Den tomme `documents`-
tabellen beholdes som kompatibilitet for import; nye data skrives aldri dit.
Et initialiseringsmerke gjør at et tomt datasett ikke importeres på nytt fra gamle
filer. En skadet database gir feil fremfor å hente en utdatert JSON-sikkerhetskopi.
Ta sikkerhetskopi av `.sqlite`-filene når appen er avsluttet.

`CUSTOM_VISNING_TASKS`, `CUSTOM_VISNING_SNOOZE` og `COPILOT_STORE_PATH` støtter
fortsatt gamle JSON-stier. Databasen ligger ved siden av filen med `.sqlite` i
stedet for `.json`. Eksplisitte `.sqlite`, `.sqlite3` og `.db`-stier brukes direkte;
andre filnavn får `.sqlite` lagt til. Mapper opprettes med 0700 og databaser med
0600. Konfigurasjonen fra `main` og Copilots kildefiler endres ikke.

### Lokal snooze

I navigasjonsmodus starter `z` snooze for valgte elementer. `zz` utvider til hele
sammenhengen og viser kjente oppgaver, Jira-saker, PR-er, AI-sesjoner og
chatmeldinger før du velger tid. Skriv eventuelt et positivt heltall og avslutt
med `h`, `d`, `m` eller `D`; dette lagrer straks. Uten tall brukes **1**:
`zh` betyr én time, `zzh` hele sammenhengen én time og `zz2h` to timer.
For eksempel betyr `z12h` tolv faktiske timer fra nå, `z3d` tre kalenderdager
med samme lokale klokkeslett og `z14m` fjorten kalenderdager fra i dag kl. 06:00.
Backspace retter tallet. Esc avbryter uten å lagre. Null, ugyldige tall og
verdier utenfor støttet tidsområde avvises. Ny snooze regnes alltid fra nå.

Listen viser antall utsatte elementer. `vz` åpner **Utsatt**, sortert etter
tidligste returtidspunkt, også for elementer utenfor gjeldende søkeresultat.
Der endrer `z` tidspunktet, `az` henter valgt element tilbake, og Esc går tilbake.
Etter lagring vises faktisk lokal returtid med tidssone og UTC-offset.

Snooze deles mellom alle faner og søk for samme tjeneste/server og URL.
En oppgaves snooze gjelder bare oppgaven; referansene beholdes og merkes hvis
referansen selv er utsatt. Med `zz` inkluderes valgte/tilknyttede oppgaver,
deres referanser og én videre relasjon til andre typer. Andre oppgaver trekkes
ikke inn via delte referanser. Fra en PR eller sak brukes direkte tilknyttede
oppgaver som utgangspunkt. Oppgaver får en varig lokal ID.

En hel sammenheng lagres som én gruppe i **Utsatt**. `d` viser medlemmene,
`z` endrer tidspunktet for gruppen og `az` henter gruppen tilbake.
Separate snoozinger og andre grupper beholdes. Nye kjente koblinger følger
pausen automatisk; tidligere kjente medlemmer beholdes også uten nettverk.
Relasjoner bygger på data appen har lastet og lokale koblinger, ikke et komplett
søk i eksterne tjenester. Delte referanser merkes som utsatt i aktive oppgaver.
GitHub og Jira endres aldri. Returtid sjekkes hvert sekund uten nettverkskall;
utløpte elementer er synlige også etter omstart og beholder vanlig sortering.

Kalenderberegning bruker maskinens lokale tidssone. Ved et manglende klokkeslett
under sommertidsovergangen flyttes tiden frem med gapet. Ved et klokkeslett som
forekommer to ganger velges første forekomst. Timer er alltid medgått tid.

Data lagres separat fra nettverkscache i `custom-visning/snooze.sqlite` under
brukerens konfigurasjonsmappe (`CUSTOM_VISNING_SNOOZE` kan overstyre filstien).
Tabellen inneholder identitet, type, tittel, URL og returtid. Gruppens utgangspunkt
og medlemmer ligger i `snooze_references`. Skriving bruker en
SQLite-transaksjon, med 0600 på databasefilen og 0700 på nye mapper.
Lagringsfeil vises, og raden skjules først når lagringen har lykkes. En skadet
snooze-database blokkerer nye lagringer til lageret er rettet og programmet startet på nytt.

## Send PR-er til Mattermost

Sett `MATTERMOST_WEBHOOK_URL` til en HTTPS incoming-webhook for ønsket kanal.
Webhook-adressen er en hemmelighet; oppbevar den utenfor kildekoden.
Installer Neovim, Vim eller vi (appen velger første tilgjengelige i den rekkefølgen).

1. Marker én eller flere PR-er med `Space`, også på tvers av faner og filtre.
   Uten markering brukes PR-en under markøren.
2. Trykk `as`. Meldingen åpnes ferdig utfylt som Markdown i Vim/Neovim.
3. Trykk `i` for å redigere. Bruk `Esc` og `:wq` for å lagre og gå tilbake.
   `:cq` avbryter redigeringen og beholder forrige utkast.
4. Se gjennom Markdown-teksten i terminalappen (`j/k` ruller), trykk `ee` for
   å redigere videre, `as` for å sende, eller `Esc` for å avbryte sendingen.

Alle valgte PR-er sendes i **én melding**, med én lenke per linje:

```markdown
[PR-tittel](https://github.com/owner/repo/pull/123)
[En annen PR-tittel](https://github.com/owner/repo/pull/124)
[JIRA-123](https://jira.example.com/browse/JIRA-123)
```

Jira-lenken legges til én gang nederst når Jira er konfigurert med
`JIRA_BASE_URL` og `JIRA_TOKEN`, og alle PR-ene har nøyaktig én og samme
Jira-nøkkel i tittel, beskrivelse eller branchnavn. Ved ulike, manglende eller
flere Jira-nøkler brukes bare PR-lenkene.

Ved sendefeil beholdes utkastet. Appen forsøker ikke automatisk på nytt;
ved nettverksfeil bør du sjekke kanalen før du sender igjen.

### GitHub-detaljer

Trykk `d` på en markert PR for å se endrede filer som et mappetre (med
endringstype, tillegg/slettinger og tidligere navn), kommentarer og reviews,
og status og resultat for hver enkelt Actions-sjekk på gjeldende PR-head.
`j`/`k` eller piltastene blar, `au` henter detaljene på nytt, `en` åpner PR-en
på GitHub, og `Esc` går tilbake til listen. Hvis et API-endepunkt ikke er
tilgjengelig, vises en advarsel sammen med detaljene som kunne lastes.

## Les Mattermost-kanaler

Åpne `vc`, velg `vs` og sett Mattermost-server og brukernavn i appen.
Alternativt kan du sette `MATTERMOST_SERVER=https://chat.example.org` og
`MATTERMOST_USERNAME` før oppstart. `vc` åpner Mattermost, `al` logger inn med passord/TOTP, `ap` bruker PAT,
og `vk` velger kanaler. Sesjoner lagres i OS-nøkkelringen når den er tilgjengelig;
meldinger og kanalvalg i SQLite gir offline-lesing. `rg` går fra en melding til en
cachet GitHub-PR, og `rc` i GitHub viser Mattermost-referansene tilbake.

Se [Mattermost-oppsett, tastatursnarveier og begrensninger](docs/mattermost.md).
