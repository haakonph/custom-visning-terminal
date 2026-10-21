package tui

import "strings"

func (m Model) keyboardHelp() string {
	if len(m.keyboard.encode) == 0 {
		return helpText + "\n\nF2  Bytt tastaturprofil; i auto knyttes profilen til aktiv inndatakilde."
	}
	rows := []string{
		"Hurtigtaster · " + m.keyboardLabel(),
		"F2  Bytt profil; auto husker profilen for hver inndatakilde.",
		"Hviletaster: QWERTY " + homeLabel(m.keyboard.options.QwertyHome) + " · eksternt " + homeLabel(m.keyboard.options.WorkmanHome),
		"Profil og hviletaster kan endres i innstillingene.",
		"",
	}
	for _, row := range []string{
		"a Handling",
		"v Visning",
		"d Detaljer",
		"e Åpne",
		"j/k/↑/↓ Naviger opp/ned",
		"h/l/←/→ Forrige/neste fane; oppgaver: forrige/neste oppgave",
		"Space Marker/fjern markering",
		"Shift+↑/↓ Utvid markeringen",
		"r Relaterte elementer",
		"t Koble elementer",
		"x Fjern alle markeringer",
		"/ Søk/filtrer; Enter bekrefter, Esc avbryter",
		"? Hjelp",
		"Esc Avbryt sekvens / tilbake",
		"Ctrl+C Avslutt",
		"vo/vg/vt/va/vc Oppgaver / GitHub / Jira / AI / chat",
		"ro/rg/rt/ra/rc Relaterte oppgaver / GitHub / Jira / AI / chat",
		"to/tg/tt/ta/tc Koble til oppgaver / GitHub / Jira / AI / chat",
		"ee/en Åpne i editor / nettleser",
		"eee/eew AI-visning: chattens arbeidsmappe / worktree",
		"ev Velg editor",
		"el Lenker fra valgte elementer",
		"as Lag/send melding; i utkastet åpner editorsekvensen Vim",
		"ak Arkiver lokalt; i arkivet: gjenopprett",
		"ad Slett lokal oppgave/referanse",
		"aw GitHub: slett lokale worktrees; chat: slutt å overvåke",
		"a+/a- Automerge på/av",
		"ar GitHub: be om ny Copilot-review",
		"atf/af Jira-flytting via relasjon / direkte på Jira",
		"q/u/a/f I Jira-overgang: QA / under arbeid / avklaring / ferdig",
		"atm/atr Tilordne meg / fjern meg fra tilhørende Jira-saker",
		"am/ar Samme tilordningshandling direkte på Jira",
		"au Oppdater/synkroniser aktiv visning",
		"an Ny intern oppgave",
		"ae Endre oppgavetittel",
		"aq/ax Lagre Jira-søk / nullstill hurtigfiltre",
		"vs Innstillinger",
		"vi Copilot-diagnostikk",
		"vr Forrige visning",
		"vh Oppsettshjelp",
		"vf/vj Jira-hurtigfiltre / JQL-redigering",
		"vz/az Vis utsatte / hent tilbake",
		"vb Lokalt arkiv",
		"vk Velg/overvåk Mattermost-kanaler",
		"al/ap Chat-innlogging med passord / PAT",
		"av/ab Merk kanal lest / tøm lokal chatkonto",
		"ah/a↓/a↑ Eldre chatmeldinger / rull en halv side",
		"z/zz Utsett valget / hele sammenhengen",
	} {
		rows = append(rows, m.keyHints(row))
	}
	rows = append(rows, "", "I tidsfeltet brukes alltid h/d/m/D og tall slik de er skrevet.",
		"Sletting av worktrees bekreftes alltid med y; n/Esc avbryter.",
		"Tekstfelt, innlimt tekst og Ctrl-snarveier følger vanlig inndata.",
		"Relasjonssekvenser kan kjedes; hjelpen i sekvensen viser neste tast.")
	return strings.Join(rows, "\n")
}

func homeLabel(home string) string {
	keys := []rune(home)
	if len(keys) >= 5 {
		return strings.ReplaceAll(string(keys[:5]), " ", "␣") + " | " + string(keys[5:])
	}
	return home
}
