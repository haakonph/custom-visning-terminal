package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func chooseAll(m Model) Model {
	for i := 0; i < len(m.filtered()); i++ {
		m, _ = update(m, key(' '))
		if i+1 < len(m.filtered()) {
			m, _ = update(m, key('j'))
		}
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
	_, cmd := sequence(m, "en")
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
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	m.tasks.path = filepath.Join(blocked, "local.sqlite")
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
