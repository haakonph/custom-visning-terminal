# Mattermost

Mattermost-leseren bruker den offisielle Go-klienten og fungerer med eller uten
GitHub. Den eksisterende incoming-webhooken for sending av PR-lenker er uendret.

## Oppsett og autentisering

Start `custom-visning`, og åpne chat med `vc`. Når oppsett mangler, vises
oppsettshjelpen. Veiviseren (`f`) ber om serveradresse og brukernavn, og åpner
vanlig innlogging med passord og eventuell TOTP-kode når oppsettet er ferdig.
PAT er valgfritt via `ap`. For å bruke innstillingene i stedet: Velg `vs`, fyll ut **Mattermost · serveradresse** og
**Mattermost · brukernavn** (ikke e-postadresse), og trykk Esc. Enter eller `vc`
tar førstegangsoppsettet i bruk uten omstart. Endring av en allerede tilkoblet
konto krever omstart. `vh` åpner oppsettshjelpen igjen fra en visning.

Alternativt kan server og brukernavn settes før oppstart:

```sh
export MATTERMOST_SERVER=https://chat.example.org
export MATTERMOST_USERNAME=ola
custom-visning
```

Serveren må bruke HTTPS; sertifikater valideres. En eventuell context path kan
inngå i URL-en. Server og brukernavn lagres i den eksisterende konfigurasjonen
(`CUSTOM_VISNING_CONFIG`, ellers OS-konfigurasjonsmappen/custom-visning/config.json):

```json
{
  "mattermost": {
    "server": "https://chat.example.org",
    "username": "ola"
  }
}
```

Miljøvariablene overstyrer filen. Etter første oppstart er de ikke nødvendige.
Bruk `vc` for Mattermost når GitHub eller Jira er aktiv. Uten GitHub åpnes
Mattermost direkte. Velg `al`, skriv passord og trykk Enter. Skriv deretter TOTP
hvis serveren krever dette, ellers trykk Enter uten kode. Ved utløpt sesjon vises
innlogging igjen; Esc lar deg fortsette å lese offline.

Alternativt: bruk `ap` og skriv et Personal Access Token i det maskerte feltet.
`MATTERMOST_TOKEN` kan også brukes; den overstyrer en lagret sesjon ved oppstart.
Tokenets bruker må ha samme brukernavn som konfigurasjonen, slik at cacher ikke
blandes mellom kontoer. Appen skriver aldri passord eller TOTP-koder til disk.

Gyldige sesjoner gjenbrukes fra macOS Keychain, Windows Credential Manager eller
Linux Secret Service (krever en tilgjengelig, opplåst nøkkelring og D-Bus).
Hvis nøkkelringen ikke er tilgjengelig, beholdes token kun i prosessminnet og du
må logge inn igjen etter omstart. Ingen hemmeligheter lagres i SQLite eller
konfigurasjonsfilen. OS kan vise sin egen nøkkelringdialog ved tilgang.

## Team, kanaler og meldinger

`vk` åpner kanalvelgeren. Den viser teamnavn og alle
offentlige kanaler i teamene dine, samt private kanaler du er medlem av. Skriv
for å søke lokalt i navn og team, bruk pilene, og Enter for å slå overvåkning
av/på. Valgene huskes per server og brukernavn. Overvåkning endrer ikke
medlemskap på serveren. En offentlig kanal kan kreve medlemskap etter serverens
tilgangsregler; slike feil vises uten å forkaste lokal historikk.

Direktemeldinger og gruppesamtaler oppdages på tvers av team og overvåkes
automatisk første gang. De kan slås av/på med `vk`, og valget huskes.
GitHub-lenker i disse meldingene indekseres på samme måte som kanalmeldinger.
Dette gjelder synkronisert historikk: de 60 siste meldingene ved første synk,
med eldre meldinger via `ah`, ikke et fullstendig søk i all DM-historikk.

Hver overvåket kanal får en horisontal tab. Tab-raden flytter seg slik at aktiv
tab er synlig. Meldinger viser tidspunkt, avsender og innhold. Overskrifter,
fet/kursiv tekst, inline-kode, kodeblokker og sitater får terminalstil; øvrig
Markdown, inkludert lenker, bevares som lesbar tekst. Ingen bilder eller vedlegg
lastes ned. `d` åpner hele tråden fra serveren og beholder svarene offline.

| Tast | Handling |
| --- | --- |
| `vc` | Åpne Mattermost fra en annen visning |
| `h` / `l`, `←` / `→` | Forrige / neste kanal |
| `j` / `k`, `↓` / `↑` | Velg melding |
| `Space`, `Shift+↑/↓`, `x` | Marker, utvid eller fjern markeringer |
| `/` | Filtrer meldinger lokalt |
| `vk` / `aw` | Velg kanaler / slutt å overvåke aktiv kanal |
| `d` | Vis tråd og svar |
| `ah` / `a↓` / `a↑` | Eldre meldinger / rull halv side ned/opp |
| `en` / `el` | Åpne meldingen i nettleser / velg lenke i meldinger |
| `rg` / `rc` | Relaterte PR-er / relaterte chatmeldinger fra GitHub |
| `to/tg/tt/ta/tc` | Koble valgte meldinger til valgte objekter lokalt |
| `as` / `ak` | Lag melding fra valget / arkiver lokalt |
| `av` / `au` | Merk kanal lest / synkroniser |
| `al` / `ap` | Logg inn med passord/TOTP / PAT |
| `ab`, så Enter | Tøm lokal chatcache og lagret sesjon |
| `Esc` / `Ctrl+C` | Tilbake/avbryt / avslutt |


Uleste meldinger markeres med `●`. Serverens sist-lest-tid hentes ved
kanaloppdagelse og kombineres med lokal lesestatus. `av` endrer bare lokal status;
appen sender ingen lesebekreftelser eller meldinger gjennom leseintegrasjonen.

## Synkronisering og offline

Data ligger i OS-cachemappen/custom-visning/mattermost.sqlite. Tabellene bruker
prosjektets eksisterende migreringssystem, med kontoavgrensning, kanaler,
meldinger, trådrøtter, synkroniseringsmarkør, lesestatus og PR-relasjoner.

Første synkronisering henter opptil 60 siste meldinger per valgt kanal. Videre
hentes endringer siden siste servermarkør med én millisekunds overlapp og
id-basert deduplisering. Redigeringer erstatter innhold; slettetombstones tømmer
lokalt innhold og fjerner PR-relasjoner. Markøren avanserer bare etter vellykket
henting og bruker serverens tidsstempler. Eldre historie hentes bare med `ah`,
og tråder ved `d`. Feil i én kanal forkaster ikke vellykket synkronisering i andre.

Automatiske oppdateringer bruker polling hvert 30. sekund, også når en annen
provider vises. WebSocket er ikke implementert. Cachen kan leses uten nett;
API-feil vises i statuslinjen. En offline sletting blir først kjent når serveren
returnerer den ved neste synkronisering; serverens retention- og tilgangsregler
kan begrense hvilke historiske endringer som er tilgjengelige.

`ab` med Enter-bekreftelse fjerner Mattermost-tabellinnhold for **alle** lokale kontoer,
kanalvalg og PR-relasjoner. Dette påvirker ikke GitHub-, Jira- eller Copilot-data.
Gjeldende kontos sesjon fjernes også fra nøkkelringen hvis den er tilgjengelig;
eventuelle nøkkelringposter for andre kontoer må fjernes i OS-nøkkelringverktøyet.
Server/brukernavn i konfigurasjonen beholdes; fjern `mattermost`-seksjonen og
miljøvariablene for å deaktivere integrasjonen helt.

## GitHub-kobling

GitHub-URL-er på formen `https://github.com/owner/repository/pull/123` gjenkjennes,
også med query, diskusjonsanker eller `/files`, `/commits` og `/checks`.
Lenkene normaliseres og dedupliseres per melding. Flere PR-er i samme melding og
samme PR i mange kanaler eller tråder støttes. Relasjoner oppdateres atomisk med
meldingene og fjernes når en melding redigeres eller slettes.

Under meldingen vises tittel, Open/Draft og review-status når PR-en finnes i den
allerede lastede GitHub-cachen. Integrasjonen gjør **ingen ekstra GitHub-kall**;
den deler snapshot og metadata med den eksisterende PR-visningen. `rg` går til
den aktuelle raden. GitHub-visningen viser antall Mattermost-referanser; `rc`
åpner referanselisten med kanal, avsender, tidspunkt og melding. Enter i valglisten går tilbake
til den nøyaktige meldingen eller dens cachede tråd.

Referanser gjelder synkronisert historikk, ikke en full serversøk-indeks. PR-er
utenfor GitHub-cachen vises som vanlige lenker. Modellen har foreløpig ingen
closed/merged-status; leseintegrasjonen viser samme Open/Draft- og review-data
som eksisterende GitHub-visninger. GitHub Enterprise-lenker støttes ikke av
PR-parseren, men kan åpnes som vanlige lenker.

## Tester

`go test ./...` og `go test -race ./...` inkluderer Mattermost-testene i CI.
Testene bruker lokale TLS-testservere, en falsk lese-API og mock-nøkkelring;
ingen ekte Mattermost-server eller konto kreves. `make lint` kjører `go vet` og
formatkontroll. SQLite-migreringene og dokumentasjonen kan kontrolleres med
`make check-generated` og `go run ./cmd/db-docs -check`.

Se [fullstendig tastaturoversikt](keyboard.md) for kjeding og alle tillegg.
