// Package local prepares safe, persistent PR worktrees and launches editors.
package local

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	appconfig "github.com/haakonph/custom-visning-terminal/internal/config"
)

type Editor struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}
type Editors struct {
	Default  string `json:"default"`
	IntelliJ Editor `json:"intellij"`
	VSCode   Editor `json:"vscode"`
	Custom   Editor `json:"custom"`
}
type Config struct {
	RepositoryPaths []string `json:"repository_paths"`
	Editors         Editors  `json:"editors"`
}

func Defaults() Config {
	return Config{Editors: Editors{Default: "intellij", IntelliJ: Editor{"idea", []string{"{path}"}}, VSCode: Editor{"code", []string{"--new-window", "{path}"}}}}
}
func Directories() (config, data string, err error) {
	config, err = os.UserConfigDir()
	if err != nil {
		return
	}
	config = appconfig.Path()
	if runtime.GOOS == "linux" {
		data = os.Getenv("XDG_DATA_HOME")
		if !filepath.IsAbs(data) {
			var home string
			home, err = os.UserHomeDir()
			if err != nil {
				return
			}
			data = filepath.Join(home, ".local", "share")
		}
	} else {
		data, err = os.UserConfigDir()
		if err != nil {
			return
		}
	}
	data = filepath.Join(data, "custom-visning")
	return
}
func LoadConfig(path string) (Config, error) {
	c := Defaults()
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, appconfig.Update(path, map[string]any{"repository_paths": c.RepositoryPaths, "editors": c.Editors})
	}
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(b, &c)
	return c, err
}

// Atomic JSON replacement follows the dashboard cache's write/rename pattern.
func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".local-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = json.NewEncoder(f).Encode(value); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}
