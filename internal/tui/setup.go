package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

type setupState struct {
	provider string
	problem  string
	offset   int
}

// WithSetup opens guidance without discarding the current provider or selection.
func (m Model) WithSetup(provider string) Model {
	m.setup = setupState{provider: provider}
	if m.source == nil {
		m.booting = false
	}
	return m
}

func (m Model) WithSetupProblem(provider, problem string) Model {
	m = m.WithSetup(provider)
	m.setup.problem = problem
	return m
}

func (m Model) setupLines() []string {
	switch m.setup.provider {
	case "github":
		return []string{
			"GitHub · innlogging og tilgang",
			"Utenfor appen: Installer GitHub CLI (gh), og logg inn i en terminal:",
			"  gh auth login --hostname github.com",
			"  gh auth refresh --hostname github.com -s repo -s read:org",
			"Autoriser SAML SSO for organisasjonen hvis den krever det.",
			"Alternativ: Sett GITHUB_TOKEN til et PAT med tilgang til repositoryene dine.",
			"GITHUB_TOKEN overstyrer gh-innlogging. Fjern en gammel verdi med unset GITHUB_TOKEN.",
			"Start custom-visning på nytt fra samme terminal etter innlogging eller tokenendring.",
			"I appen: vg åpner GitHub, au henter data på nytt. Ved tilgangsfeil: kontroller token, SSO og repository-tilgang.",
		}
	case "jira":
		return []string{
			"Jira · server og personlig tilgangstoken",
			"I appen: vs åpner innstillinger. Fyll ut Jira · serveradresse, f.eks. https://jira.example.no/jira.",
			"Utenfor appen: Opprett et personlig tilgangstoken (PAT) i Jira-profilen. Administrator må ha tillatt PAT.",
			"Integrasjonen bruker Jira Data Center / Server med Bearer-token og REST API v2.",
			"Les inn token i Bash uten å vise det eller legge det i historikken:",
			`  read -rsp 'Jira PAT: ' JIRA_TOKEN; echo; export JIRA_TOKEN`,
			"Start custom-visning på nytt fra samme terminal. Miljøvariabler fra andre terminaler følger ikke med.",
			"I appen: vt åpner Jira, au henter data på nytt. Ved tilgangsfeil: sjekk token og prosjekttilgang.",
		}
	case "mattermost":
		return []string{
			"Mattermost · lese kanaler og logge inn",
			"I appen: vs åpner innstillinger. Fyll ut Mattermost · serveradresse (HTTPS) og Mattermost · brukernavn.",
			"Bruk Mattermost-brukernavnet, ikke e-postadressen. Serveradressen kan inneholde en context path.",
			"Ved første oppsett: Trykk Esc fra innstillingene, deretter Enter eller vc for å ta det i bruk uten omstart.",
			"Endring av en allerede tilkoblet konto krever omstart.",
			"I chatten: al åpner innlogging med passord og eventuell TOTP-kode. ap åpner et maskert PAT-felt.",
			"Utenfor appen, hvis du bruker PAT: Opprett token i Mattermost-profilen; administrator må ha tillatt dette.",
			"vk velger kanaler når du er innlogget. En lagret sesjon gjenbrukes fra operativsystemets nøkkelring.",
			"Linux: Nøkkelring krever tilgjengelig, opplåst Secret Service og D-Bus. Uten nøkkelring må du logge inn etter omstart.",
			"MATTERMOST_SERVER og MATTERMOST_USERNAME overstyrer innstillingene; fjern gamle verdier i startterminalen ved behov.",
			"Ingen webhook kreves for lesing. Sending av PR-lenker bruker et separat webhook-oppsett (as viser hjelp hvis det mangler).",
		}
	case "copilot":
		return []string{
			"AI / Copilot · lokal historikk og valgfri telemetri",
			"Utenfor appen: Installer Copilot CLI, logg inn der og start en sesjon. Historikk leses fra ~/.copilot/session-state.",
			"I appen: vs lar deg endre sesjonsmappe og innsamling. Start appen på nytt etter endringer.",
			"va åpner AI, au skanner på nytt, vi viser diagnostikk. Ingen GitHub-token kreves for lokal historikk.",
			"Valgfri telemetri: Sett følgende i terminalen som starter Copilot eller IDE-en:",
			"  export COPILOT_OTEL_ENABLED=true",
			"  export COPILOT_OTEL_CAPTURE_CONTENT=true",
			"  export OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318",
			"  export OTEL_EXPORTER_OTLP_PROTOCOL=http/json",
			"La custom-visning kjøre mens Copilot sender. En IDE som allerede kjører må startes på nytt for å arve miljøvariabler.",
			"Ved portkonflikt: Velg en ledig OTLP-port i innstillingene, start appen på nytt og bruk samme port i exporter-endpointet.",
			"Innholdsinnsamling kan inkludere samtaler og verktøydata. Kontroller innstillingene for lagring og visning av innhold.",
		}
	case "webhook":
		return []string{
			"Mattermost · sende meldinger og PR-lenker",
			"Utenfor appen: Opprett en incoming-webhook for ønsket kanal i Mattermost. Administrator må ha tillatt integrasjonen.",
			"Les inn hele HTTPS-adressen i Bash uten å vise den eller legge den i historikken:",
			`  read -rsp 'Mattermost webhook: ' MATTERMOST_WEBHOOK_URL; echo; export MATTERMOST_WEBHOOK_URL`,
			"Start custom-visning på nytt fra samme terminal.",
			"I appen: Marker elementene du vil dele og bruk as. Gjennomgå meldingen før du sender med as fra utkastet.",
			"Webhook-adressen er en hemmelighet og skal ikke legges i config.json. Chat-innlogging aktiverer ikke denne webhooken.",
		}
	case "editor":
		return []string{
			"Editor og lokale repositories",
			"Utenfor appen: Installer Git og editoren du vil bruke. Sørg for at git og editorens launcher er tilgjengelige.",
			"I appen: vs åpner repository paths og editorinnstillinger (IntelliJ, VS Code eller egen kommando).",
			"Repository paths er absolutte stier til lokale repositories. Bruk editorens kommando og argumenter ved behov.",
			"Repoer fra PR-listene klargjøres på main i bakgrunnen, med Maven/Gradle/npm fra prosjektets rot.",
			"IntelliJ headless · kommando/argumenter i vs aktiverer indeksering før åpning. Java/Node må være installert.",
			"Git må ha tilgang til private repositories via SSH eller din Git-credential-håndtering. API-token alene konfigurerer ikke Git.",
			"Start appen på nytt etter endringer. ee åpner valgt element i editor, ev velger editor.",
		}
	default:
		return []string{
			"Lokalt oppsett",
			"Oppgaver, koblinger, utsettelse og arkiv lagres lokalt og krever ingen ekstern innlogging.",
			"vs åpner innstillingene. vg GitHub · vt Jira · va AI · vc Mattermost viser hjelp når oppsett mangler.",
			"For editor og lokale repositories: Åpne GitHub, og bruk ee; vs lar deg velge repository paths og editor.",
		}
	}
}

func (m Model) setupBody() []string {
	var body []string
	if m.setup.problem != "" {
		body = append(body, strings.Split(ansi.Hardwrap(clean(m.setup.problem), max(1, m.width), true), "\n")...)
		body = append(body, "")
	}
	for _, line := range m.setupLines() {
		body = append(body, strings.Split(ansi.Hardwrap(clean(line), max(1, m.width), true), "\n")...)
	}
	return body
}

func (m Model) setupView() tea.View {
	body := m.setupBody()
	capacity := max(1, m.height-2)
	start := min(m.setup.offset, max(0, len(body)-capacity))
	lines := []string{accent.Render(" OPPSETTSHJELP · " + m.setup.provider)}
	lines = append(lines, body[start:min(len(body), start+capacity)]...)
	lines = append(lines, "vs innstillinger · vg/vt/va/vc sider · j/k bla · Enter prøv igjen · Esc tilbake")
	if len(lines) > m.height {
		lines = lines[:max(1, m.height)]
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], max(1, m.width), "…")
	}
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	return view
}

func (m Model) updateSetup(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "v":
		m.sequence = "v"
		m.sequenceRefs = nil
	case "?":
		m.help = true
		m.helpOffset = 0
	case "j", "down":
		m.setup.offset = min(m.setup.offset+1, max(0, len(m.setupBody())-max(1, m.height-2)))
	case "k", "up":
		m.setup.offset = max(0, m.setup.offset-1)
	case "pgdown":
		m.setup.offset = min(m.setup.offset+max(1, m.height-2), max(0, len(m.setupBody())-max(1, m.height-2)))
	case "pgup":
		m.setup.offset = max(0, m.setup.offset-max(1, m.height-2))
	case "esc":
		if m.source != nil || m.jiraActive || m.copilotActive || m.tasks.active || (m.mmReader != nil && m.mmReader.active) {
			m.setup = setupState{}
		}
	case "enter":
		key := map[string]string{"github": "g", "jira": "t", "mattermost": "c", "copilot": "a"}[m.setup.provider]
		if key != "" {
			return m.viewProvider(key)
		}
	}
	return m, nil
}
