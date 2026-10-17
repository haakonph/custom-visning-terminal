package model

import (
	"regexp"
	"strings"
)

var jiraKeyPattern = regexp.MustCompile(`[A-Z][A-Z0-9_]*-[0-9]+`)

// JiraKeys returns distinct issue keys in encounter order. Match whole tokens
// so a key such as ABC-12 never matches ABC-123 or part of another word.
func (p PullRequest) JiraKeys() []string {
	text := p.Title + "\n" + p.Body + "\n" + p.HeadRef
	var keys []string
	seen := map[string]bool{}
	for _, loc := range jiraKeyPattern.FindAllStringIndex(text, -1) {
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

func (p PullRequest) HasJiraKey(key string) bool {
	for _, k := range p.JiraKeys() {
		if k == strings.ToUpper(key) {
			return true
		}
	}
	return false
}
