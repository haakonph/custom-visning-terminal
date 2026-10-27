package snooze

import (
	"errors"
	"fmt"
	"github.com/haakonph/custom-visning-terminal/internal/storage"
	"os"
)

type Store struct{ Path string }

func (s Store) Load() (map[string]Entry, error) {
	entries := map[string]Entry{}
	err := storage.Load(s.Path, &entries)
	if errors.Is(err, os.ErrNotExist) {
		return entries, nil
	}
	if err != nil {
		return nil, err
	}
	for id, e := range entries {
		if id == "" || e.ID != id || e.Until.IsZero() || e.Until.Year() > 9999 || (e.Kind != "github" && e.Kind != "jira" && e.Kind != "task" && e.Kind != "copilot" && e.Kind != "mattermost") {
			return nil, fmt.Errorf("ugyldig snooze-data")
		}
	}
	if err := storage.Migrate(s.Path, entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (s Store) Save(entries map[string]Entry) error {
	return storage.Save(s.Path, entries)
}
