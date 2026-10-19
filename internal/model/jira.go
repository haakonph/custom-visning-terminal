package model

import (
	"regexp"
	"strings"
)

var jiraKeyPattern = regexp.MustCompile(`[A-Z][A-Z0-9_]*-[0-9]+`)

// JiraKeys returns distinct issue keys in encounter order. Match whole tokens
// so a key such as ABC-12 never matches ABC-123 or part of another word.
func (p PullRequest) JiraKeys(patterns ...*regexp.Regexp) []string {
	pattern := jiraKeyPattern
	if len(patterns) > 0 && patterns[0] != nil {
		pattern = patterns[0]
	}
	text := p.Title + "\n" + p.Body + "\n" + p.HeadRef
	var keys []string
	seen := map[string]bool{}
	for _, loc := range pattern.FindAllStringIndex(text, -1) {
		if loc[0] == loc[1] {
			continue
		}
		if loc[0] > 0 && jiraWord(text[loc[0]-1]) || loc[1] < len(text) && jiraWord(text[loc[1]]) {
			continue
		}
		key := text[loc[0]:loc[1]]
		if !seen[key] {
			keys = append(keys, key)
			seen[key] = true
		}
	}
	return keys
}

func jiraWord(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}

func (p PullRequest) HasJiraKey(key string, patterns ...*regexp.Regexp) bool {
	for _, k := range p.JiraKeys(patterns...) {
		if k == strings.ToUpper(key) {
			return true
		}
	}
	return false
}

var emptyJiraBrackets = regexp.MustCompile(`\[[\s,;/|]*\]|\{[\s,;/|]*\}|\([\s,;/|]*\)`)

// DisplayTitle removes whole Jira keys and empty wrappers for presentation only.
func (p PullRequest) DisplayTitle(patterns ...*regexp.Regexp) string {
	pattern := jiraKeyPattern
	if len(patterns) > 0 && patterns[0] != nil {
		pattern = patterns[0]
	}
	var title strings.Builder
	start := 0
	for _, loc := range pattern.FindAllStringIndex(p.Title, -1) {
		if loc[0] == loc[1] || loc[0] > 0 && jiraWord(p.Title[loc[0]-1]) || loc[1] < len(p.Title) && jiraWord(p.Title[loc[1]]) {
			continue
		}
		title.WriteString(p.Title[start:loc[0]])
		start = loc[1]
	}
	if start == 0 {
		return p.Title
	}
	title.WriteString(p.Title[start:])
	text := title.String()
	for {
		next := emptyJiraBrackets.ReplaceAllString(text, "")
		if next == text {
			break
		}
		text = next
	}
	return strings.Trim(strings.Join(strings.Fields(text), " "), " :;,-–—|")
}
