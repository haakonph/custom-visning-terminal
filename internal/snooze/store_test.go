package snooze

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDeadlinesAndDST(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Oslo")
	if err != nil { t.Fatal(err) }
	for _, tc := range []struct { name, start, digits, unit, want string }{
		{"hours", "2026-03-28T12:00:00+01:00", "24", "h", "2026-03-29T13:00:00+02:00"},
		{"spring calendar", "2026-03-28T12:00:00+01:00", "1", "d", "2026-03-29T12:00:00+02:00"},
		{"spring six", "2026-03-28T12:00:00+01:00", "1", "D", "2026-03-29T06:00:00+02:00"},
		{"spring gap", "2026-03-28T02:30:00+01:00", "1", "d", "2026-03-29T03:30:00+02:00"},
		{"fall calendar", "2026-10-24T12:00:00+02:00", "1", "d", "2026-10-25T12:00:00+01:00"},
		{"fall overlap", "2026-10-24T02:30:00+02:00", "1", "d", "2026-10-25T02:30:00+02:00"},
		{"fall hours", "2026-10-24T12:00:00+02:00", "24", "h", "2026-10-25T11:00:00+01:00"},
		{"multi digit", "2026-10-18T12:34:56+02:00", "14", "D", "2026-11-01T06:00:00+01:00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start, err := time.Parse(time.RFC3339, tc.start)
			if err != nil { t.Fatal(err) }
			got, err := Deadline(start.In(loc), tc.digits, tc.unit)
			if err != nil || got.Format(time.RFC3339) != tc.want { t.Fatalf("got %v (%v), want %s", got, err, tc.want) }
		})
	}
	for _, digits := range []string{"", "0", "-1", "1.5", "x", "18446744073709551616", "18446744073709551615"} {
		for _, unit := range []string{"h", "d", "D"} {
			if _, err := Deadline(time.Now(), digits, unit); err == nil { t.Errorf("accepted %q%s", digits, unit) }
		}
	}
	if _, err := Deadline(time.Now(), "1", "x"); err == nil { t.Fatal("accepted invalid unit") }
}

func TestStoreRestartExpiryAndPermissions(t *testing.T) {
	s := Store{Path: filepath.Join(t.TempDir(), "data", "snooze.json")}
	now := time.Now()
	entries := map[string]Entry{
		"github:a": {ID: "github:a", Kind: "github", Title: "Retained metadata", Until: now.Add(time.Hour)},
		"jira:b": {ID: "jira:b", Kind: "jira", Title: "Expired", Until: now.Add(-time.Hour)},
	}
	if err := s.Save(entries); err != nil { t.Fatal(err) }
	loaded, err := (Store{Path: s.Path}).Load()
	if err != nil { t.Fatal(err) }
	rows := Active(loaded, now)
	if len(rows) != 1 || rows[0].Title != "Retained metadata" { t.Fatal(rows) }
	if len(Active(loaded, now.Add(time.Hour))) != 0 { t.Fatal("deadline must expire at equality") }
	info, err := os.Stat(s.Path)
	if err != nil { t.Fatal(err) }
	if info.Mode().Perm() != 0600 { t.Fatal(info.Mode()) }
	if err := os.WriteFile(s.Path, []byte("broken"), 0600); err != nil { t.Fatal(err) }
	if _, err := s.Load(); err == nil { t.Fatal("corrupt file accepted") }
}

func TestServiceAndServerIdentity(t *testing.T) {
	a := Identity("jira", "https://one.example/jira/browse/ABC-1")
	if a == Identity("jira", "https://two.example/jira/browse/ABC-1") || a == Identity("github", "https://one.example/jira/browse/ABC-1") { t.Fatal("identity collision") }
	if a != Identity("jira", "https://ONE.example/jira/browse/ABC-1/?foo=bar#x") { t.Fatal("identity is not canonical") }
}
