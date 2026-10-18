// Package config owns the application's shared configuration file.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func Path() string {
	if path := os.Getenv("CUSTOM_VISNING_CONFIG"); path != "" {
		return path
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "custom-visning", "config.json")
}

func Read(path string) (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]json.RawMessage{}, nil
	}
	if err != nil {
		return nil, err
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, errors.New("configuration must be a JSON object")
	}
	return doc, nil
}

func Section(path, key string, target any) error {
	doc, err := Read(path)
	if err != nil {
		return err
	}
	if raw, ok := doc[key]; ok {
		return json.Unmarshal(raw, target)
	}
	return nil
}

// Update preserves other settings, including fields from newer app versions.
func Update(path string, values map[string]any) error {
	doc, err := Read(path)
	if err != nil {
		return err
	}
	for key, value := range values {
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		doc[key] = raw
	}
	return Write(path, doc)
}

func Write(path string, value any) error {
	if path == "" {
		return errors.New("configuration path unavailable")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".config-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return fmt.Errorf("save configuration: %w", err)
	}
	return nil
}
