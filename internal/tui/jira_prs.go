package tui

import (
	"fmt"
	"strings"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

type jiraPRSummary struct {
	count       int
	status      string
	explanation string
}

// The snapshot contains open PRs from the configured GitHub views, not merge history.
func (m Model) jiraPRSummary(key string) jiraPRSummary {
	prs := m.relatedPRs(key)
	if m.source == nil && len(prs) == 0 {
		return jiraPRSummary{status: "Ukjent", explanation: "GitHub er ikke konfigurert; antall åpne PR-er er ukjent."}
	}
	if len(prs) == 0 {
		return jiraPRSummary{}
	}
	labels := []string{"Blokkert", "Endringer ønsket", "Venter på review", "Under arbeid", "Sjekker pågår", "Ukjent", "Klar til merge"}
	counts := make([]int, len(labels))
	for _, p := range prs {
		i := 5
		switch {
		case p.Mergeability == model.Conflicting || p.CI == model.Fail:
			i = 0
		case p.Review == model.ChangesRequested:
			i = 1
		case p.Draft:
			i = 3
		case p.Review == model.NoReview || p.Review == model.Pending:
			i = 2
		case p.CI == model.Pending:
			i = 4
		case p.Review == model.Approved && p.CI == model.Pass && p.Mergeability == model.Mergeable:
			i = 6
		}
		counts[i]++
	}
	summary := jiraPRSummary{count: len(prs)}
	var parts []string
	descriptions := []string{"har mergekonflikt eller feilede sjekker", "har fått endringer ønsket", "venter på review", "er utkast", "venter på sjekker", "mangler statusdata", "er klare til merge"}
	for i, count := range counts {
		if count == 0 {
			continue
		}
		if summary.status == "" {
			summary.status = labels[i]
		}
		parts = append(parts, fmt.Sprintf("%d %s", count, descriptions[i]))
	}
	summary.explanation = strings.Join(parts, "; ") + ". Gjelder PR-er i GitHub-visningene."
	return summary
}

func (s jiraPRSummary) countLabel() string {
	if s.status == "Ukjent" && s.count == 0 {
		return "?"
	}
	if s.count == 0 {
		return ""
	}
	return fmt.Sprint(s.count)
}

func (s jiraPRSummary) detail() string {
	if s.countLabel() == "" {
		return ""
	}
	return fmt.Sprintf("PR-er: %s åpne · %s · %s", s.countLabel(), s.status, s.explanation)
}
