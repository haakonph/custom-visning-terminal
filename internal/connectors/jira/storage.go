package jira

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type Query struct {
	Name string `json:"name"`
	JQL  string `json:"jql"`
}
type Config struct {
	Queries []Query `json:"queries"`
}

func ConfigPath() string {
	if path := os.Getenv("JIRA_CONFIG"); path != "" {
		return path
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "custom-visning", "jira.json")
}
func LoadConfig(path string) (Config, error) {
	var c Config
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(data, &c)
	if err != nil {
		return Config{}, err
	}
	if err == nil {
		for _, q := range c.Queries {
			if q.Name == "" || q.JQL == "" {
				return Config{}, errors.New("saved queries require name and JQL")
			}
		}
	}
	return c, err
}
func writeJSON(path string, v any) error {
	if path == "" {
		return errors.New("storage path unavailable")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".jira-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = json.NewEncoder(f).Encode(v); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
func SaveConfig(path string, c Config) error { return writeJSON(path, c) }

type Cached struct {
	Query    string
	Issues   []Issue
	SyncedAt time.Time
}

func (c *Client) cachePath(query string) string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	sum := sha256.Sum256([]byte(c.base.String() + "\x00" + c.token + "\x00" + query))
	return filepath.Join(dir, "custom-visning", "jira", hex.EncodeToString(sum[:])+".json")
}
func (c *Client) LoadCache(query string) (Cached, error) {
	var data Cached
	b, err := os.ReadFile(c.cachePath(query))
	if err != nil {
		return data, err
	}
	err = json.Unmarshal(b, &data)
	if err == nil && data.Query != query {
		return Cached{}, errors.New("cache query mismatch")
	}
	return data, err
}
func (c *Client) SaveCache(data Cached) error { return writeJSON(c.cachePath(data.Query), data) }
