package jira

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"

	appconfig "github.com/haakonph/custom-visning-terminal/internal/config"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/storage"
)

type Query struct {
	Name string `json:"name"`
	JQL  string `json:"jql"`
}
type Config struct {
	Queries []Query `json:"queries"`
	Pattern string  `json:"pattern,omitempty"`
	BaseURL string  `json:"base_url,omitempty"`
	Project string  `json:"project,omitempty"`
}

func ConfigPath() string { return appconfig.Path() }

// Migrate imports legacy Jira settings once, leaving the original file intact.
func Migrate() error {
	doc, err := appconfig.Read(ConfigPath())
	if err != nil {
		return err
	}
	if _, ok := doc["jira"]; ok {
		return nil
	}
	path := os.Getenv("JIRA_CONFIG")
	if path == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		path = filepath.Join(dir, "custom-visning", "jira.json")
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return err
	}
	if err := validateConfig(c); err != nil {
		return err
	}
	return SaveConfig(ConfigPath(), c)
}
func LoadConfig(path string) (Config, error) {
	var c Config
	if err := appconfig.Section(path, "jira", &c); err != nil {
		return c, err
	}
	return c, validateConfig(c)
}
func validateConfig(c Config) error {
	for _, q := range c.Queries {
		if q.Name == "" || q.JQL == "" {
			return errors.New("saved queries require name and JQL")
		}
	}
	if c.Pattern != "" {
		if _, err := regexp.Compile(c.Pattern); err != nil {
			return errors.New("invalid Jira pattern: " + err.Error())
		}
	}
	return nil
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
func SaveConfig(path string, c Config) error {
	if err := validateConfig(c); err != nil {
		return err
	}
	return appconfig.Update(path, map[string]any{"jira": c})
}

type Cached = model.JiraCache

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
	err := storage.Load(c.cachePath(query), &data)
	if err != nil {
		return data, err
	}
	if data.Query != query {
		return Cached{}, errors.New("cache query mismatch")
	}
	if err = storage.Migrate(c.cachePath(query), data); err != nil {
		return Cached{}, err
	}
	return data, nil
}
func (c *Client) SaveCache(data Cached) error { return storage.Save(c.cachePath(data.Query), data) }
