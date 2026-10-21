package tui

import (
	"embed"
	"os"
	"path/filepath"
)

//go:embed notification_icons/*.png
var notificationIcons embed.FS

func notificationStyle(status string) (icon, badge string) {
	switch status {
	case "PASS", "Approval":
		return "success", "✅"
	case "FAIL", "Changes":
		return "failure", "❌"
	case "Review", "Dismissed":
		return "warning", "⚠️"
	default:
		return "info", "ℹ️"
	}
}

func notificationIconPath(name string) (string, error) {
	data, err := notificationIcons.ReadFile("notification_icons/" + name + ".png")
	if err != nil {
		return "", err
	}
	root, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, "custom-visning", "notification-icons")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	// Publish atomically so concurrent notifications never read a partial PNG.
	f, err := os.CreateTemp(dir, ".icon-*.png")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name+".png")
	if err := os.Rename(f.Name(), path); err != nil {
		return "", err
	}
	return path, nil
}
