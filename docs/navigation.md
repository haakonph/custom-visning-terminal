# Forslag: felles navigasjon i terminalappen

Status: konseptforslag, ikke implementert. Omfanget er navigasjon mellom dagens
områder, med plass til detaljer, flere filtre og koblinger mellom alle typer.
Forslaget tar utgangspunkt i hva brukeren skal finne og gjøre; dagens hurtigtaster
og inndeling er ikke føringer for løsningen.

## Anbefaling

Bruk én felles modell: **område → visning → element → relasjon**. Alle områder
bruker samme tastaturregler, skjermramme og returmekanisme. Områdene gir faste
innganger; relasjonene lar brukeren følge arbeidet på tvers av dem.

**Prioritet: navigering mellom koblede elementer skal være rask og identisk
overalt.** Fra både liste og detaljer åpner `l` samme relasjonsvelger for
fokusert element. Enter følger valgt kobling. Esc går tilbake til nøyaktig
utgangspunkt med rad, filter og rulleposisjon bevart. Brukeren skal ikke måtte
gå via områdevelger eller finne en Relasjoner-seksjon for hvert hopp.

Fra k9s henter vi kompakte ressurslister, synlig kontekst, filtrering og direkte
bytte av område via kommando. Fra Vim henter vi bevegelse, tydelig skille mellom
navigasjon og tekstinntasting og historikk tilbake/frem. Full Vim-emulering er
ikke et mål; få regler skal kunne læres én gang og brukes overalt.

## Dagens innhold som skal være tilgjengelig

Kartlagt mot `origin/main` på commit `2a115b2`, særlig `internal/tui/keys.go`,
`update.go`, `details.go`, `jira.go`, `tasks.go`, `copilot.go`, `related.go`,
`snooze.go` og `settings.go`.

| Område | Innhold som må kunne nås | Rolle i forslaget |
| --- | --- | --- |
| GitHub | Egne åpne PR-er, drafts, direkte review, teamreview; PR-filer, sjekker og kommentarer | PR-lister med navngitte visninger og PR-detaljer |
| Jira | Mine, prosjekt, uten ansvarlig, aktiv sprint, lagrede søk og egen JQL | Sakslister med navngitte visninger og saksdetaljer |
| Oppgaver | Lokale oppgaver og referanser til Jira/PR | Oppgaveliste og detaljer med relasjoner |
| Copilot | Sesjoner, samtale/hendelser, forbruk og diagnostikk | Sesjonsliste og detaljer; diagnostikk som egen underside |
| Utsatt | Utsatte PR-er, Jira-saker og oppgaver, med returtid | Felles liste som åpner elementets vanlige detaljer |
| Innstillinger | Appkonfigurasjon og oppsett | Fast systemområde med samme fokus- og returregler |
| Handlinger | Nettleser, editor/worktree, opprette/koble oppgave, snooze og Mattermost | Handlingsmeny på element eller eksplisitt utvalg |

Lokale repoer og worktrees er foreløpig kontekst for handlinger, ikke et nytt
toppområde. Mattermost-utkast er en arbeidsflyt med retur til utgangspunktet.
Forslaget oppretter ikke nye datakilder eller relasjoner som appen ennå ikke har.

I dag åpner Enter nettleseren i GitHub/Jira, detaljer i Copilot og referanser i
oppgaver. Tab bytter Jira-visning, men velger oppgave i oppgaveområdet. Esc kan
både fjerne filter og lukke detaljer. Relasjonshopp erstatter deler av målområdets
søk. Dette er forskjeller den nye modellen skal fjerne.

## Områder, visninger og detaljer

**Område** velger hva slags innhold du arbeider med: GitHub, Jira, Oppgaver,
Copilot, Utsatt eller Innstillinger. En felles områdevelger åpnes fra alle sider.
Alle innganger er synlige også uten konfigurasjon; da vises forklaring og vei til
oppsett fremfor en tom liste eller en snarvei som ikke virker.

**Visning** er et navngitt utvalg i området. «Mine åpne», «Teamreview» og et lagret
Jira-søk er visninger, ikke egne navigasjonssystemer. En søkbar visningsvelger
skalerer til mange lagrede visninger; skjermbredden skal ikke begrense antallet.

**Element** åpner alltid en detaljside i appen. Detaljer følger samme ramme selv
om innholdet varierer. Fremtidige detaljsider kan begynne med tilgjengelig
metadata og relasjoner; full innhenting av alt innhold er ikke en forutsetning.

**Relasjon** åpner et koblet element eller en avgrenset liste av koblede elementer.
Et hopp fra Jira til PR-er skal åpne «PR-er knyttet til APP-123», uten å endre
brukerens vanlige GitHub-visning. Flere treff vises som liste; ett treff i en
relasjonstype åpnes også via listen slik at resultat og avgrensning er tydelige.
En eksplisitt valgt elementlenke åpner detaljene direkte.

## Felles skjermramme

```text
 CUSTOM VISNING   GitHub / Teamreview                  NORMAL
 Visning: Teamreview        Synk: 10:35 · delvis
 Filtre: tekst="login" · repo=frontend       12 av 84 lastede
 ──────────────────────────────────────────────────────────
 REPOSITORY          PR     TITTEL              REVIEW
 > acme/frontend     #123   Fix login flow      PENDING
   acme/frontend     #456   Add passkey          APPROVED
 ──────────────────────────────────────────────────────────
 Enter detaljer · / søk · f filtre · v visning · : kommando
```

Detaljsiden erstatter hovedinnholdet og beholder ramme, status og hjelpelinje.
Den viser elementidentitet og seksjoner som Oversikt, Innhold og Relasjoner,
med domenespesifikke seksjoner som Filer og Sjekker der det passer.
Navigasjonshistorikken er separat fra plasseringen: «GitHub / PR #123» sier
hva som er åpent, mens «Tilbake til Jira / APP-123 / Relasjoner» sier hvor du kom fra.

Begynn med én hovedflate. Brede terminaler kan senere vise en forhåndsvisning
ved siden av listen, men Enter skal fortsatt åpne detaljer og Tab bytte fokus.
På smale terminaler prioriteres identitet, tittel, aktivt fokus og filterindikator;
lange stier og hjelpetekst forkortes. `?` viser alltid full hjelp.

## Tastaturkontrakt

Tabellen gjelder navigasjonsmodus. I tekstfelt skrives bokstavene som tekst.

| Tast | Felles betydning |
| --- | --- |
| `j/k`, `↓/↑` | Flytt i aktiv liste eller rull aktivt lesepanel |
| `gg` / `G` | Første / siste rad eller begynnelsen / slutten av tekst |
| `Enter` | Åpne valgt element internt, følge valgt lenke eller aktivere felt |
| `Esc` | Avbryt aktiv inntasting, ellers lukk øverste dialog, ellers gå tilbake ett nivå |
| `Ctrl+O` / `Ctrl+I` | Tilbake / frem i besøkshistorikken i navigasjonsmodus |
| `Tab` / `Shift+Tab` | Neste / forrige fokusområde; aldri bytte datasett |
| `[` / `]` | Forrige / neste seksjon på en detaljside |
| `v` | Velg navngitt visning i gjeldende område |
| `l` | Åpne relasjonsvelger for fokusert element, fra både liste og detaljer |
| `:` | Felles søkbar kommando- og områdevelger |
| `/` | Rediger tekstfilter i aktiv liste; søk i tekst når et lesepanel har fokus |
| `f` | Åpne strukturerte filtre for aktiv liste |
| `Space` | Marker element i en liste som støtter flervalg |
| `a` | Vis tilgjengelige handlinger og hvilket element/utvalg de gjelder |
| `o` | Åpne fokusert element eksternt; deaktiveres med forklaring uten ekstern lenke |
| `r` | Oppdater aktivt innhold, uten å nullstille navigasjon eller filtre |
| `?` | Hjelp for gjeldende side og de felles tastene |
| `q` | Avslutt fra navigasjonsmodus; uferdige utkast får valg om å beholde eller forkaste |

`Ctrl+I` og Tab kan være samme tast i terminalen. Tab beholder alltid
fokusfunksjonen; «Historikk: frem» må derfor også finnes i kommandovelgeren.
Ingen nødvendig navigasjon avhenger av at terminalen skiller dem.

Kommandovelgeren viser for eksempel «Gå til Jira», «Gå til Oppgaver»,
«GitHub: Teamreview», «Fjern tekstfilter», «Nullstill filtre» og «Avslutt».
Korte aliaser som `:jira`, `:prs`, `:tasks`, `:copilot`, `:snoozed` og `:settings`
kan brukes av erfarne brukere. `:jql …` er en Jira-spesifikk kommando; `:` alene
betyr det samme overalt. Tall kan senere være valgfrie snarveier, men er ikke
grunnlaget for å nå alle visninger.

Esc fjerner aldri et bekreftet filter. I filterredigering gjenoppretter Esc
verdien fra før redigeringen; Enter bekrefter. På en detaljside går Esc tilbake
til listen med samme filtre og markerte element. På områdets rotliste er Esc
uten effekt og hjelpelinjen tilbyr områdevelgeren. Historikk tilbake brukes
for å returnere til et tidligere område.

Flervalg er eksplisitt og lokalt for listekonteksten. Antall markerte og antall
skjulte markerte vises. Enter og `o` gjelder alltid fokusert rad. Massehandlinger
velges gjennom `a`, som viser omfanget før handlingen utføres. Dette unngår at
«åpne detaljer» plutselig åpner mange nettleserfaner.

## Filtrering som kan vokse

Skill mellom fire ting i alle lister:

1. Grunnutvalget fra den navngitte visningen, for eksempel Teamreview.
2. Relasjonsavgrensning, for eksempel «knyttet til APP-123», når listen er åpnet slik.
3. Strukturerte filtre via `f`, med felter tilpasset innholdet.
4. Lokalt tekstfilter via `/`, blant elementene som er hentet.

Vis lagene samlet og gjør hvert filter mulig å fjerne separat. «Nullstill filtre»
fjerner brukerens strukturerte filtre og tekstfilter, men endrer ikke visning
eller relasjonsavgrensning. «Lagre som visning» bevarer den eksplisitte
søkeoppskriften; lokale tekstfiltre blir ikke automatisk til serverfiltre.

Samme filterdialog brukes overalt, med ulike felt: repo, review og CI for PR;
prosjekt, ansvarlig og status for Jira; prosjekt, modell og tidspunkt for Copilot;
referansetype for oppgaver; type og returtid for Utsatt. Feltene er fremtidige
muligheter der datastøtte mangler, ikke løfter om funksjonalitet i dag.

Filtre tilhører den enkelte visningen. De arves ikke skjult mellom områder eller
vanlige visninger. Relasjonslister har egen filtertilstand, slik at et gammelt
GitHub-filter ikke skjuler koblingene fra Jira. Et eventuelt felles arbeidsområde
på tvers av tjenester må senere velges eksplisitt og vises i rammen.

Angi alltid om filtrering gjelder lastede data eller et søk mot tjenesten.
«0 treff blant 84 lastede» er forskjellig fra «ingen resultater i tjenesten».
Ved delvise resultater og feil beholdes liste og filtre, med synlig status og
mulighet til å prøve igjen. Ikke hent fra nettverket for hvert tastetrykk i `/`.

## Relasjoner og retur

Alle detaljsider får en Relasjoner-seksjon med type, antall og datagrunnlag.
`l` gir en direktevei til de samme koblingene fra alle elementlister og
detaljsider, inkludert Utsatt. Velgeren viser en flat, søkbar liste av konkrete
koblede elementer, gruppert visuelt etter type. `j/k` velger, tekst snevrer inn,
Enter åpner interne detaljer, og Esc avbryter. Ved tekstinntasting i velgeren
er bokstaver alltid søketekst; piltastene flytter valg. For større eller delvis
lastede grupper finnes «Vis alle relaterte PR-er/saker/…», som åpner den
avgrensede relasjonslisten. Velgeren har samme rekkefølge og regler uansett
hvilken tjeneste kildeelementet kommer fra. Handlingen gjelder fokusert
element selv om flere rader er markert.

Kjente koblinger vises straks fra tilgjengelige data. Eventuell innhenting skjer
i bakgrunnen og merkes som pågående; eksisterende treff kan åpnes mens den
pågår. Ingen ny henting skal være nødvendig for å gå tilbake. Etterlasting
bevarer valgt identitet og flytter ikke markøren. Nettverksavhengige oppslag
kan ta tid, men skal ikke blokkere tastatur, tilbake eller allerede kjente treff.

Dagens eksplisitte oppgavereferanser og utledede PR–Jira-koblinger kan vises her.
Fremtidige koblinger til sesjoner, repoer og worktrees bruker samme navigasjon
når datagrunnlaget finnes. Skill synlig mellom eksplisitt kobling og utledet treff.

Relaterte PR-er som bare finnes blant lastede GitHub-data merkes nettopp slik.
«Ingen kjente koblinger», «ikke konfigurert», «laster» og «kunne ikke hente» er
ulike tilstander. En kobling til et element som ikke lenger kan hentes, viser
lagret identitet og forklaring; tilbakefunksjonen skal fortsatt virke.

Hver historikkoppføring bevarer område, visning, elementidentitet,
relasjonskontekst, filtre, sortering, fokus, seksjon og rulleposisjon. Bevar valgt
rad etter stabil identitet, ikke indeks. Hvis raden er borte etter oppdatering,
velges nærmeste gjenværende rad og endringen forklares. Ny navigasjon etter et
tilbakehopp erstatter fremoverhistorikken. Å flytte markøren lager ikke historikk.

Det skilles mellom områdets sist brukte vanlige visning og midlertidige
relasjonsbesøk. «Gå til GitHub» åpner sist brukte vanlige GitHub-visning;
historikk tilbake går til det faktiske forrige besøket. Hoppet krever ikke at
data er ferdig lastet. Forsinkede svar får ikke flytte brukerens fokus eller
erstatte en annen besøkskontekst.

## Eksempler som modellen skal støtte

| Forløp | Forventet resultat |
| --- | --- |
| Jira / Mine → filtrer → Enter på APP-123 → Relasjoner → PR-er → PR #456 → Esc → Esc → Esc | Tilbake i Jira-detaljene med samme seksjon; videre Esc gir opprinnelig Jira-liste med filter og rad bevart |
| GitHub / Teamreview → filter repo → `:tasks` → åpne oppgave → følg PR-referanse | PR-detaljer åpnes uten å endre Teamreview-filteret; historikk tilbake returnerer til oppgaven |
| Copilot → åpne sesjon → innstillinger → Esc | Samme sesjon, seksjon og rulleposisjon vises igjen |
| Utsatt → åpne PR → tilbake → hent PR tilbake fra snooze | Utsatt-listen oppdateres; fokus flyttes forutsigbart når raden forsvinner |
| `/` → skriv `jira` → Esc | Teksten behandles som filtertekst, ingen områdeendring; tidligere filter gjenopprettes |
| Åpne relaterte PR-er når GitHub ikke er konfigurert | Forklaring og inngang til oppsett; retur til kildesiden virker |
| Aktivt filter gir null treff | Filtrene blir stående med «0 treff» og synlig handling for å endre eller fjerne dem |
| PR-rad → `l` → Jira-sak → `l` → oppgave → Esc → Esc | Hvert hopp bruker samme velger; retur gir først Jira-saken og så opprinnelig PR-liste, med tilstanden bevart |
| `l` fra samme element i liste, detaljer og Utsatt | Samme koblinger, rekkefølge og taster; kjente treff kan brukes uten å vente på nettverk |

## Avgrenset første gjennomføring

Første implementasjon bør etablere rammen og navigasjonskontrakten på dagens
innhold: områdevelger, felles fokusregler, internt Enter, separat ekstern åpning,
tilbakehistorikk og tilstand per visning. Dagens detaljer gjenbrukes som innhold.
Den felles `l`-velgeren for eksisterende koblinger er del av denne første
gjennomføringen, ikke en senere utvidelse.
Jira og Utsatt kan først vise tilgjengelig metadata og referanser i samme ramme.
Handlingene som allerede finnes flyttes inn i den felles handlingsmenyen.

Deretter kan filterdialog, lagrede visninger og relasjonslister bygges ut uten
å endre den grunnleggende navigasjonen. Fullverdige detaljsider for alle typer,
nye koblinger mellom datakilder, globalt søk og flere samtidige paneler ligger
utenfor første gjennomføring.

Før kodeendringen regnes som ferdig, må eksempelforløpene over verifiseres,
inkludert smal terminal, asynkron oppdatering, tom liste og feil ved lasting.
Målet er at brukeren alltid kan se hvor de er, hva et filter gjelder, hva Enter
vil åpne, og hvordan de kommer tilbake.
