package jira

import (
	"context"
	"fmt"
	"strings"
)

type WorkflowAPI interface {
	Transitions(context.Context, string) ([]Transition, error)
	Assign(context.Context, string, bool) error
	Transition(context.Context, string, string) error
}
type WorkflowResult struct {
	Transitions map[string][]Transition
	Updated     []string
	Err         error
}

func PrepareWorkflow(ctx context.Context, client WorkflowAPI, keys []string, operation string) WorkflowResult {
	result := WorkflowResult{Transitions: map[string][]Transition{}}
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			result.Err = err
			break
		}
		if operation == "f" {
			result.Transitions[key], result.Err = client.Transitions(ctx, key)
		} else {
			result.Err = client.Assign(ctx, key, operation == "m")
		}
		if result.Err != nil {
			result.Err = fmt.Errorf("%s: %w (tidligere saker kan være oppdatert)", key, result.Err)
			break
		}
		if operation != "f" {
			result.Updated = append(result.Updated, key)
		}
	}
	return result
}
func TransitionName(t Transition) string {
	if t.To.Name != "" {
		return t.To.Name
	}
	return t.Name
}
func CommonTransitions(keys []string, transitions map[string][]Transition) []string {
	if len(keys) == 0 {
		return nil
	}
	var choices []string
	seen := map[string]bool{}
	for _, transition := range transitions[keys[0]] {
		name := TransitionName(transition)
		common := name != "" && !seen[name]
		for _, key := range keys {
			count := 0
			for _, t := range transitions[key] {
				if TransitionName(t) == name {
					count++
				}
			}
			if count != 1 {
				common = false
			}
		}
		if common {
			choices = append(choices, name)
			seen[name] = true
		}
	}
	return choices
}
func ExecuteTransitions(ctx context.Context, client WorkflowAPI, keys []string, transitions map[string][]Transition, choice string) WorkflowResult {
	result := WorkflowResult{}
	// Resolve every target before the first network mutation.
	ids := map[string]string{}
	for _, key := range keys {
		matches := 0
		for _, t := range transitions[key] {
			if TransitionName(t) == choice {
				ids[key] = t.ID
				matches++
			}
		}
		if matches != 1 {
			result.Err = fmt.Errorf("%s: ingen entydig overgang til %s", key, choice)
			return result
		}
	}
	for _, key := range keys {
		if err := client.Transition(ctx, key, ids[key]); err != nil {
			result.Err = fmt.Errorf("%s: %w (tidligere saker kan være oppdatert)", key, err)
			return result
		}
		result.Updated = append(result.Updated, key)
	}
	return result
}
func StatusMatches(name, shortcut string) bool {
	aliases := map[string][]string{"q": {"qa", "quality assurance", "klar for qa", "ready for qa"}, "u": {"under arbeid", "in progress"}, "a": {"avklaring", "clarification", "needs clarification"}, "f": {"ferdig", "done", "closed"}}
	for _, alias := range aliases[shortcut] {
		if strings.EqualFold(strings.TrimSpace(name), alias) {
			return true
		}
	}
	return false
}
