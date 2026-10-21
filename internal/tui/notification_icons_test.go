package tui

import (
	"bytes"
	"image/png"
	"os"
	"reflect"
	"testing"
)

func TestNotificationStatusIcons(t *testing.T) {
	for _, tc := range []struct{ status, icon, badge string }{
		{"PASS", "success", "✅"}, {"Approval", "success", "✅"},
		{"FAIL", "failure", "❌"}, {"Changes", "failure", "❌"},
		{"Review", "warning", "⚠️"}, {"Dismissed", "warning", "⚠️"},
		{"Comments", "info", "ℹ️"}, {"", "info", "ℹ️"},
	} {
		icon, badge := notificationStyle(tc.status)
		if icon != tc.icon || badge != tc.badge {
			t.Fatalf("%s: %s %s", tc.status, icon, badge)
		}
		data, err := notificationIcons.ReadFile("notification_icons/" + icon + ".png")
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil || img.Bounds().Dx() != 64 || img.Bounds().Dy() != 64 {
			t.Fatal("invalid status image", err)
		}
	}
}

func TestNotificationIconArguments(t *testing.T) {
	event := desktopNotification{title: "CI", body: "PR", url: "https://github.com/a/b/pull/1", icon: "/tmp/status icons/success.png"}
	_, args := notificationCommand("darwin", event)
	if !reflect.DeepEqual(args[len(args)-2:], []string{"-appIcon", "file:///tmp/status%20icons/success.png"}) {
		t.Fatal(args)
	}
	_, args = notificationCommand("linux", event)
	if args[4] != "--icon="+event.icon || args[5] != "--" {
		t.Fatal(args)
	}
}

func TestNotificationIconCache(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	path, err := notificationIconPath("success")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if _, err := notificationIconPath("success"); err != nil {
		t.Fatal("replacing cached icon", err)
	}
}
