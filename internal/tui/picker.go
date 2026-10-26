package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Picker text takes precedence over single-letter navigation shortcuts.
func pickerQuery(key tea.KeyPressMsg, query *string) bool {
	if key.String() == "backspace" {
		runes := []rune(*query)
		if len(runes) > 0 {
			*query = string(runes[:len(runes)-1])
		}
		return true
	}
	if key.Text != "" && key.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModSuper) == 0 {
		*query += key.Text
		return true
	}
	return false
}

func (d *entityDialog) typeToSearch() bool {
	return d.mode == "link" || d.mode == "related" || d.mode == "external"
}

func (l *localState) editorChoices() []string {
	var choices []string
	for _, name := range []string{"IntelliJ IDEA", "VS Code", "egendefinert"} {
		if strings.Contains(strings.ToLower(name), strings.ToLower(l.editorQuery)) {
			choices = append(choices, name)
		}
	}
	return choices
}
