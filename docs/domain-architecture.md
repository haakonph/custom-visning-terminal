# Domener og arbeidsflyter

Appen er én modulær Go-applikasjon med Bubble Tea og SQLite. Domenene eier
arbeidsreglene. Terminalen velger mål, viser resultater og holder navigasjon,
filtre og utkast. Endringer i minnet anvendes sekvensielt; nettverk og disk kjøres
som bakgrunnsarbeid med uforanderlige inngangsdata.

## Moduler

| Pakke | Ansvar |
| --- | --- |
| `pullrequests` | Autoritativt PR-register, listemedlemskap, automerge og review-forespørsler |
| `pullrequests/github` | GitHub-API, autentisering og oppretting av klient og tilhørende cache |
| `tasks` | Oppgaveidentitet, oppgavereferanser og målrettede endringer med forventet revisjon |
| `relations` | Relasjonsindeks, koblinger, arkiv og regler for å fjerne relasjoner |
| `jira` | Jira-integrasjonen, søk og regler for felles overganger og sekvensielle handlinger |
| `conversations` | Mattermost-kontoer, kanaler, meldinger, synkronisering og deling |
| `copilot` | Lokal historikk, sesjoner, import, telemetri og livssyklusen til mottakeren |
| `snooze` | Identitet, returtider, grupper og endringer i utsettelser |
| `local` | Lokale repositories, arbeidsmapper og editorer |
| `app` | Arbeidsflyter som krysser lokale domener og en felles transaksjonsgrense |
| `localstate` | Typer for en transaksjon på tvers av lokale domener |
| `syncstate` | Øktgenerasjoner, forespørselsidentitet, kansellering og tidsstyring etter feil |
| `storage` | SQLite, SQL-operasjoner og migreringer |
| `tui` | Tastatur, primærside, dialoger og leseprojeksjoner for presentasjon |
| `bootstrap` | Setter sammen integrasjonene, tjenestene, lagringen og terminalprogrammet |

`app` importerer ikke `tui` eller Bubble Tea. De selvstendige PR-handlingene og
Jira-arbeidsreglene kan testes med falske klienter uten terminalmodell.
Felles referansetyper ligger foreløpig i `model`, slik at en oppgave kan peke på
et eksternt element uten å være avhengig av dets cache eller klient.

## Én PR-identitet per økt

`pullrequests.State` lagrer PR-er etter URL, som inkluderer tjenesteverten.
Listene lagrer bare identiteter og tidspunkt for siste fullstendige resultat.
Markeringer lagrer ID-er, og detaljvisningen lagrer valgt ID. Data slås opp i
registeret når de brukes. Registeret beholder også elementer som fortsatt kan
være valgt eller åpne etter at de har forsvunnet fra en liste.

Snapshotet som listene leser, er en uforanderlig projeksjon, ikke en ny skrivekilde.
`Export` lager en dyp kopi før asynkron cachelagring. En automerge-handling endrer
registeret én gang; alle berørte visninger leser dermed samme status.

Batchhandlinger dedupliserer målene før første API-kall, kjøres sekvensielt og
stopper ved første feil. Resultatet skiller mellom vellykkede, feilede og ikke
forsøkte mål. Delvis suksess beholdes. Handlinger med usikkert nettverksutfall
forsøkes ikke automatisk igjen.

## Økter og forsinkede svar

En forespørsel får generasjon, sekvensnummer og revisjon når den starter.
Omkonfigurering kansellerer gammel økt og øker generasjonen. Et sent svar må
fortsatt valideres selv om forespørselen er kansellert.

- Generasjon skiller kontoer og konfigurasjoner.
- Sekvensnummer skiller forespørsler for samme ressurs eller spørring.
- Revisjon hindrer eldre lesinger i å overskrive en nyere handling.

PR-registeret sammenligner også observasjonsrekkefølgen på tvers av lister og
detaljforespørsler. Et eldre svar for én liste får ikke erstatte nyere PR-data fra
en annen liste. Et delvis listesvar oppdaterer observerte elementer og bevarer
øvrig medlemskap. Bare et fullstendig svar erstatter listemedlemskapet.

Uavhengige skrivehandlinger bruker forespørselsidentitet ved fullføring, slik at
en review-forespørsel og automerge kan bli ferdige i vilkårlig rekkefølge.
Klient og cachemål fanges ved oppstart av bakgrunnsjobben. Cachelagring er
serialisert også når økten erstattes, slik at en eldre skriving ikke kan avslutte
etter en nyere skriving til samme fil.

## Lokale data og konfliktkontroll

Oppgaver, oppgavereferanser, koblinger, arkiv, snooze og sendetider for review-
meldinger ligger i det felles lokale lageret. Nettverkscache og Copilot-historikk
har sine eksisterende grenser. Tømming av nettverkscache påvirker ingen av disse
lokale oppgavene eller relasjonene.

Oppgaver har stabil ID og en revisjon som økes ved hver målrettet oppdatering.
Oppretting, oppdatering og sletting skriver bare berørte oppgaver. En
SQLite-transaksjon kontrollerer forventet revisjon og innhold før skriving.
Endringer fra andre prosesser i uberørte oppgaver beholdes. Konflikt i samme
oppgave avviser hele kommandoen, inkludert tilhørende kobling eller arkivering.

Koblinger, arkiv og snooze bruker målrettede endringer med kontroll av forventet
innhold. Idempotente like tillegg godtas. En kommando som berører flere lokale
domener har én commit. Terminalen trenger ingen kompenserende snapshots etter
delvis lagring. Nye arkiv-, koblings- og snooze-resultater vises etter vellykket
commit. Et oppgaveutkast som ikke kunne lagres, blir værende i minnet, også når en
annen lokal handling fullføres. `au` på oppgavesiden leser lokale data på nytt i
bakgrunnen.

`storage.Save` er beholdt for cache og kompatibilitetsimport. Vanlige lokale
arbeidsflyter bruker `TaskStore` og `LocalStore` med konkrete endringsoperasjoner.

## Migrering og lagringsstier

Standardmålet er det eksisterende oppgavelageret, normalt
`os.UserConfigDir()/custom-visning/tasks.sqlite`. `CUSTOM_VISNING_TASKS` bevarer
sin betydning for standardplasseringen. `CUSTOM_VISNING_LOCAL` kan velge et
annet samlet lager. JSON-stier normaliseres til SQLite-stien som før.

`CUSTOM_VISNING_LINKS` og `CUSTOM_VISNING_SNOOZE` angir kildene som skal importeres
ved oppgraderingen. Ved egen `CUSTOM_VISNING_LOCAL` importeres også oppgavene fra
`CUSTOM_VISNING_TASKS`. Kildestier beholdes i migreringsmetadata. Endring av en
allerede importert kildesti krever eksplisitt avklaring, slik at et eksisterende
lokalt datasett aldri erstattes av en ny, gammel kilde.

Hvert domene importeres i en transaksjon sammen med importmerket. En avbrutt
import kan prøves igjen. Et tomt importert datasett er også initialisert og blir
ikke fylt fra kilden igjen etter senere sletting. Eksisterende lokale data
beholdes også når en gammel kilde mangler. Review-sendetider følger med fra det
gamle koblingslageret, også når det ikke inneholder koblinger.

Originale JSON-filer beholdes uendret. SQLite-kilder oppgraderes gjennom ordinære
skjemamigreringer. Eldre appversjoner som ikke kjenner skjemaet, avviser dermed
skriving. Ikke kjør gammel og ny appversjon samtidig under oppgraderingen; ta
sikkerhetskopi før en eventuell nedgradering. Et nyere eller korrupt skjema blir
rapportert som feil. Kildens innhold kontrolleres ved senere oppstart; endringer
etter import gir feil fremfor å bli oversett eller overskrive nyere lokale data.

## Terminalgrensen

Én `page` angir aktiv primærside. Dialoger, innstillinger og andre overlays er
separate fra primærsiden og beholder sine egne felt. Interne operasjoner kaller
navngitte handlinger direkte, uten syntetiske tastetrykk eller rekursiv `Update`.
Bare ekte tastetrykk registreres som brukeraktivitet.

Domeneresultater behandles før inputruting til aktiv side eller dialog. Nettverks-
svar og lagringsresultater kan derfor fullføres mens et tekstfelt, en dialog eller
oppsettsveiviseren er åpen.

## Kontroller

`go test ./...`, `go test -race ./...`, `make lint`, `make check-generated` og
`go build ./...` er de vanlige kontrollene. Regresjonstestene dekker blant annet
uavhengige prosesser, revisjonskonflikter, feil mellom lokale deloperasjoner,
gjentatt migrering, gamle svar etter kontobytte og resultater bak overlays.
