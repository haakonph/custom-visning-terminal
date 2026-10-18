// Package snooze stores local deferrals independently of provider caches.
package snooze

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Entry struct {
	ID    string    `json:"id"`
	Kind  string    `json:"kind"`
	Title string    `json:"title"`
	URL   string    `json:"url,omitempty"`
	Until time.Time `json:"until"`
}

// Identity includes the service and server (including a Jira installation path).
func Identity(kind, link string) string {
	u, err := url.Parse(link)
	if err != nil || u.Host == "" {
		return ""
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.RawQuery, u.Fragment, u.RawFragment = "", "", ""
	u.Path = strings.TrimRight(u.Path, "/")
	return kind + ":" + u.String()
}

// Deadline uses elapsed hours, but civil calendar arithmetic for d and D.
// Nonexistent local times advance by the DST gap; ambiguous times use the
// earlier occurrence. This policy is independent of time.Date's DST choice.
func Deadline(now time.Time, digits, unit string) (time.Time, error) {
	n, err := strconv.ParseUint(digits, 10, 64)
	if err != nil || n == 0 {
		return time.Time{}, errors.New("skriv et positivt heltall")
	}
	var at time.Time
	switch unit {
	case "h":
		if n > uint64((1<<63-1)/int64(time.Hour)) {
			return at, errors.New("for mange timer")
		}
		at = now.Add(time.Duration(n) * time.Hour)
	case "d", "D":
		if n > 3652059 {
			return at, errors.New("for mange dager")
		}
		h, min, sec := now.Clock()
		ns := now.Nanosecond()
		if unit == "D" {
			h, min, sec, ns = 6, 0, 0, 0
		}
		civil := time.Date(now.Year(), now.Month(), now.Day()+int(n), h, min, sec, ns, time.UTC)
		if civil.Year() > 9999 {
			return at, errors.New("tidspunktet er utenfor gyldig område")
		}
		at = localCivil(civil, now.Location())
	default:
		return at, errors.New("bruk h, d eller D")
	}
	if !at.After(now) || at.Year() > 9999 {
		return time.Time{}, errors.New("tidspunktet er utenfor gyldig område")
	}
	return at, nil
}

func localCivil(civil time.Time, loc *time.Location) time.Time {
	guess := time.Date(civil.Year(), civil.Month(), civil.Day(), civil.Hour(), civil.Minute(), civil.Second(), civil.Nanosecond(), loc)
	// Discover offsets on both sides of a transition, including non-hour gaps.
	offsets := map[int]bool{}
	for _, d := range []time.Duration{-48 * time.Hour, 0, 48 * time.Hour} {
		_, offset := guess.Add(d).Zone()
		offsets[offset] = true
	}
	var exact, next time.Time
	var nextCivil time.Time
	for offset := range offsets {
		candidate := civil.Add(-time.Duration(offset) * time.Second).In(loc)
		wall := time.Date(candidate.Year(), candidate.Month(), candidate.Day(), candidate.Hour(), candidate.Minute(), candidate.Second(), candidate.Nanosecond(), time.UTC)
		if wall.Equal(civil) && (exact.IsZero() || candidate.Before(exact)) {
			exact = candidate
		}
		if wall.After(civil) && (next.IsZero() || wall.Before(nextCivil)) {
			next, nextCivil = candidate, wall
		}
	}
	if !exact.IsZero() {
		return exact
	}
	if !next.IsZero() {
		return next
	}
	return guess
}

type Store struct{ Path string }

func (s Store) Load() (map[string]Entry, error) {
	entries := map[string]Entry{}
	b, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return entries, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(b, &entries); err != nil {
		return nil, err
	}
	for id, e := range entries {
		if id == "" || e.ID != id || e.Until.IsZero() || e.Until.Year() > 9999 || (e.Kind != "github" && e.Kind != "jira" && e.Kind != "task") {
			return nil, fmt.Errorf("ugyldig snooze-data")
		}
	}
	return entries, nil
}

func (s Store) Save(entries map[string]Entry) error {
	if s.Path == "" {
		return errors.New("mangler filsti")
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.Path), ".snooze-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = json.NewEncoder(f).Encode(entries); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), s.Path)
}

func Active(entries map[string]Entry, now time.Time) []Entry {
	var rows []Entry
	for _, e := range entries {
		if e.Until.After(now) {
			rows = append(rows, e)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Until.Equal(rows[j].Until) {
			return rows[i].ID < rows[j].ID
		}
		return rows[i].Until.Before(rows[j].Until)
	})
	return rows
}
