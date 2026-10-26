package tui

const shortcuts = "j/k ↑/↓ velg · h/l ←/→ fane · d detaljer · Space marker · r relatert · v visning · t koble · e åpne · a handling · / søk · vh oppsett · ? hjelp · Ctrl+C avslutt"
const helpText = `Hurtigtaster
j/k / ↑/↓   Naviger opp/ned
h/l / ←/→   Forrige/neste fane; oppgaver: forrige/neste oppgave
d           Detaljer; chat: tråd
Space       Marker/fjern markering, i alle fem typer
Shift+↑/↓   Utvid markeringen med aktiv og neste/forrige rad
x           Fjern alle markeringer; behold søk og filtre
/           Søk/filtrer; Enter beholder søket, Esc avbryter
?           Vis/lukk hjelp
Esc         Avbryt sekvens, lukk detaljer eller fjern markering
z / zz      Utsett valgte elementer / hele sammenhengen
            h timer / d kalenderdager / m eller D dager kl. 06
            Uten tall: 1. zh = én time; zzh = hele sammenhengen én time
            zz viser kjente relasjoner før du velger tid; Esc avbryter

Ting: o oppgaver · g GitHub · t Jira · a AI/Copilot · c chat/Mattermost
vo/vg/vt/va/vc  Visningsside
vp              Repoer med skrivetilgang: / søk · Enter/en GitHub · au oppdater
vl              Lokale mapper: Alle / Favoritter / Skjulte
                af favoritt · ah skjul/vis · ee editor · au oppdater
aw / ee         Jira: opprett / åpne arbeidsmappe; velg repo med skrivetilgang
ro/rg/rt/ra/rc  Relaterte elementer av valgt type
                Flere treff vises i en søkbar valgliste
                Skriv for å søke, ↑/↓ velg, Enter går til treffet
                Tab bytter til handlinger med r/e/a/t og navigasjon med j/k
to/tg/tt/ta/tc  Koble til valgte mål av typen; Enter lagrer lokalt
                Space og Shift+↑/↓ velger flere kilder og mål
ee/en           Editor (IntelliJ) / nettleser
eee/eew         AI-visning: chattens arbeidsmappe / worktree i IntelliJ
e + ting + e/n  Følg relasjon(er), åpne i editor/nettleser
as/ak/ad        Melding / lokalt arkiv / slett lokal oppgave/referanse
aw              GitHub: slett valgte lokale worktrees; y bekrefter, Esc avbryter
a+/a-          GitHub: automerge på / av for valgte PR-er
                Merge when ready: GitHub merger når kravene er oppfylt
                AUTO viser bare ON når automerge er på; QUEUE viser mergekøen
ar              GitHub: be om ny Copilot-review for valgte PR-er
                Også etter tidligere review; GitHub må ha Copilot-tilgang
atf             Flytt tilhørende Jira-saker; velg faktisk overgang
atfq/atfu       QA / under arbeid
atfa/atff       Avklaring / ferdig
atm/atr         Tilordne meg / fjern meg som ansvarlig
af/am/ar        Samme Jira-handlinger direkte på Jira-saker
                Målene tas fra valget ved starten av sekvensen
                q/u/a/f kan skrives mens overganger lastes
                Esc avbryter før utføring; feil stopper resten
                Tidligere saker i flervalget kan være oppdatert

aa… / ac…       Følg AI/chat, avslutt med s melding eller k arkiv
Eksempler: atgs, etn, aas, atgtfq (hver ting følger én relasjon)

Tillegg for funksjoner som ikke var definert i listen:
au              Oppdater/synkroniser aktiv visning eller detaljer
an/ae           Ny intern oppgave fra valget / endre oppgavetittel
aq/ax           Lagre Jira-søk / nullstill hurtigfiltre og egen JQL
vs/vi/vr        Innstillinger / Copilot-diagnostikk / forrige visning
vm              Lokal bruksstatistikk; h/l velger 7 dager / 30 dager / alt
vh              Oppsettshjelp for aktiv side
vf/vj           Jira-hurtigfiltre / JQL-redigering
vz/az           Vis utsatte / hent tilbake valgte utsatte elementer
vb              Lokalt arkiv; ak gjenoppretter valgte elementer
vk              Velg/overvåk Mattermost-kanaler
al/ap           Chat-innlogging med passord+TOTP / PAT
av/aw/ab        Merk kanal lest / slutt å overvåke / tøm lokal chatkonto
ah/a↓/a↑        Eldre chatmeldinger / rull en halv side ned/opp
el              Velg lenker fra valgte elementer
ev             Velg editor med j/k + Enter (IntelliJ, VS Code, egendefinert)
Ctrl+C          Avslutt og kanseller bakgrunnsarbeid
Enter           Bekreft i dialoger, valglister og tekstfelt

Gamle enkelttaster og nummer-/Tab-snarveier er fjernet.
Tekstfelt mottar bokstavene som tekst; de starter ikke tastsekvenser.
Koblinger og arkivering er lokale. ad sletter ikke eksterne ressurser.
AI-sesjoner uten HTTPS-adresse åpnes i editor, ikke nettleser.
Jira-status må være tilgjengelig og entydig; andre ansvarlige fjernes ikke.
UNKNOWN betyr at GitHub ikke har levert en status.
Delvis oppdatering beholder cache og tidspunkt for siste fulle synk.`
