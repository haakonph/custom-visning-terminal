package mattermost

import (
	"os"
	"strings"

	"github.com/haakonph/custom-visning-terminal/internal/config"
)

// Config contains only public settings. Passwords, TOTP codes and tokens must
// never be added to the shared configuration document.
type Config struct {
	Server   string `json:"server"`
	Username string `json:"username"`
}

func LoadConfig() (Config, error) {
	var c Config
	if err := config.Section(config.Path(), "mattermost", &c); err != nil {
		return c, err
	}
	if value := os.Getenv("MATTERMOST_SERVER"); value != "" {
		c.Server = value
	}
	if value := os.Getenv("MATTERMOST_USERNAME"); value != "" {
		c.Username = value
	}
	c.Server = strings.TrimRight(strings.TrimSpace(c.Server), "/")
	c.Username = strings.TrimSpace(c.Username)
	return c, nil
}
func SaveConfig(c Config) error { return config.Update(config.Path(), map[string]any{"mattermost": c}) }
