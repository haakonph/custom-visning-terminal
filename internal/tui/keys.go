package tui

const shortcuts = "j/k ↑/↓ velg · h/l ←/→ fane · d detaljer · Space marker · r relatert · v visning · t koble · e åpne · a handling · / søk · ? hjelp · R oppdater · q avslutt"
const helpText = `Hurtigtaster (i navigasjonsmodus)
j/k / ↑/↓   Naviger opp/ned
h/l / ←/→   Forrige/neste fane (oppgaver: forrige/neste oppgave)
d           Detaljer (Mattermost: tråd)
Space       Marker/fjern markering av PR eller Jira-sak
Shift+↑/↓   Utvid markering fra aktiv rad, inkludert neste/forrige rad
x           Fjern alle markeringer (beholder søk og filtre)
/           Søk/filtrer; Enter beholder, Esc avbryter søk
?           Vis/lukk denne hjelpen
Esc         Avbryt sekvens, lukk detaljer eller fjern markeringer
z           Utsett: positivt heltall, så h timer / d dager / D kl. 06
Z           Vis utsatte; z endrer tidspunkt, u gjenoppretter

Starttast + ting: o oppgave, g GitHub, t Jira, a Copilot, c Mattermost
vo/vg/vt/va/vc  Gå til visningsside
rt / rg     Relatert Jira fra PR / relaterte PR-er fra Jira
rc          Mattermost-omtaler av aktiv PR
to / tn     Koble valgte PR-er/Jira-saker til aktiv / ny intern oppgave
ee / en     Åpne PR-er i standardeditor / åpne i nettleser
etn / egn   Åpne tilknyttede Jira-saker / relaterte lastede PR-er i nettleser
ac          Rediger valgte PR-er som Markdown og send til Mattermost
atf         Flytt tilknyttede Jira-saker: velg faktisk overgang
atfq        Flytt til QA (også direkte fra GitHub, uten visningsbytte)
atfu/atfa/atff  Under arbeid / avklaring / ferdig
atm / atr   Tilordne Jira til meg / fjern meg som ansvarlig
            I Jira kan a + f/m/r brukes uten t
            f viser liste; j/k + Enter eller 1–9 velger overgang
            q/u/a/f velger entydig status; Esc avbryter før utføring
            Alle valgte PR-ers Jira-nøkler dedupliseres
            Mislykket flervalg kan ha oppdatert tidligere saker

Oppgaver: d detaljer, ad fjern referanse i detaljer / slett oppgave i liste
          aa arkiver/gjenopprett, A bytt aktiv/arkiv, E endre tittel, n ny
Copilot:  D diagnostikk, ee IntelliJ
Mattermost: A innlogging, d tråd, H eldre meldinger, R synk
Jira:     f hurtigfiltre, : JQL, s lagre søk; Space markerer saker
R         Oppdater aktiv visning / prøv GitHub-oppstart igjen
1–4       GitHub-fane; 1–9 Jira-fane
, / P     Innstillinger
Enter     Åpne valgt element (oppgaver: detaljer)
q/Ctrl+C  Avslutt (q er QA bare inne i flyttesekvensen)

Snarveier som ikke støttes for valgt element gir en melding.
Tekstfelt, snooze, editorvalg og dialoger beholder egen tastaturstyring.
UNKNOWN betyr at GitHub ikke har levert en status.
Delvis oppdatering beholder cache og tidspunkt for siste fulle synk.`
