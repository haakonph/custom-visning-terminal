package config

import (
	"os"
	"path/filepath"
)

func localDefault(env, name string) string {
	if path := os.Getenv(env); path != "" {
		return path
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "custom-visning", name)
}
func TaskPath() string   { return localDefault("CUSTOM_VISNING_TASKS", "tasks.json") }
func LinkPath() string   { return localDefault("CUSTOM_VISNING_LINKS", "links.sqlite") }
func SnoozePath() string { return localDefault("CUSTOM_VISNING_SNOOZE", "snooze.json") }

// LocalPath defaults to the existing task store, preserving its configured
// location. Other legacy paths are import sources, not independent write targets.
func LocalPath() string {
	if path := os.Getenv("CUSTOM_VISNING_LOCAL"); path != "" {
		return path
	}
	return TaskPath()
}
