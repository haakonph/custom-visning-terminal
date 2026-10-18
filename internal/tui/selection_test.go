package tui

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func chooseAll(m Model) Model {
	for _, p := range m.filtered() {
		m.chosen[p.Key()] = p
	}
	return m
}

func TestBrowserOpensAllChosenDespiteFilterAndFailure(t *testing.T) {
	m := chooseAll(snoozeReady(t))
	m.input.SetValue("no match")
	var urls []string
	m.open = func(_ context.Context, url string) error {
		urls = append(urls, url)
		if len(urls) == 1 {
			return errors.New("first failed")
		}
		return nil
	}
	_, cmd := update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	msg := cmd().(openMsg)
	if len(urls) != 2 || urls[0] == urls[1] || msg.err == nil {
		t.Fatalf("urls=%v error=%v", urls, msg.err)
	}
}

func TestSnoozeAllChosenAndClearSelection(t *testing.T) {
	m := chooseAll(snoozeReady(t))
	m.input.SetValue("no match")
	m = press(m, "z2h")
	if len(m.snoozes.entries) != 2 || len(m.chosen) != 0 {
		t.Fatal("not all selections snoozed")
	}
	var deadline string
	for _, e := range m.snoozes.entries {
		if deadline != "" && deadline != e.Until.String() {
			t.Fatal("different deadlines")
		}
		deadline = e.Until.String()
	}
	saved, err := m.snoozes.store.Load()
	if err != nil || len(saved) != 2 {
		t.Fatalf("saved=%v error=%v", saved, err)
	}
}

func TestSnoozeBatchFailureRetainsSelectionAndEntries(t *testing.T) {
	m := chooseAll(snoozeReady(t))
	// A file as the parent prevents creation without depending on permissions.
	m.snoozes.store.Path = filepath.Join(m.tasks.path, "snooze.json")
	m.tasks.save()
	m = press(m, "z2h")
	if len(m.snoozes.entries) != 0 || len(m.chosen) != 2 || !m.snoozes.input {
		t.Fatal("failed save changed state")
	}
}

func TestCollectAllChosenPRs(t *testing.T) {
	for _, create := range []bool{true, false} {
		m := chooseAll(snoozeReady(t))
		next, _ := m.collectTask(create)
		m = next.(Model)
		if len(m.tasks.items) != 1 || len(m.tasks.items[0].References) != 2 {
			t.Fatal("missing selected references")
		}
	}
}
