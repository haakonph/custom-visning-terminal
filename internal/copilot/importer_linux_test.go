//go:build linux

package copilot

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// Observe actual kernel open events, so opening then immediately closing an
// unchanged file cannot pass the regression test.
func watchSourceOpens(t *testing.T, directories ...string) func() map[string]int {
	t.Helper()
	fd, err := unix.InotifyInit1(unix.IN_NONBLOCK | unix.IN_CLOEXEC)
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EPERM) {
		t.Skip("inotify unavailable in this environment")
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { unix.Close(fd) })
	watches := map[int]string{}
	for _, path := range directories {
		wd, err := unix.InotifyAddWatch(fd, path, unix.IN_OPEN)
		if err != nil {
			t.Fatal(err)
		}
		watches[wd] = path
	}
	return func() map[string]int {
		t.Helper()
		opened := map[string]int{}
		buffer := make([]byte, 64<<10)
		for {
			n, err := unix.Read(fd, buffer)
			if errors.Is(err, unix.EINTR) {
				continue
			}
			if errors.Is(err, unix.EAGAIN) {
				return opened
			}
			if err != nil {
				t.Fatal(err)
			}
			for offset := 0; offset < n; {
				wd := int(int32(binary.NativeEndian.Uint32(buffer[offset : offset+4])))
				mask := binary.NativeEndian.Uint32(buffer[offset+4 : offset+8])
				length := int(binary.NativeEndian.Uint32(buffer[offset+12 : offset+16]))
				next := offset + unix.SizeofInotifyEvent + length
				name := strings.TrimRight(string(buffer[offset+unix.SizeofInotifyEvent:next]), "\x00")
				if mask&unix.IN_OPEN != 0 && mask&unix.IN_ISDIR == 0 {
					opened[filepath.Join(watches[wd], name)]++
				}
				offset = next
			}
		}
	}
}

func TestScanOpensOnlyNewOrChangedSourceFiles(t *testing.T) {
	o := testOptions(t)
	oldDir, activeDir := filepath.Join(o.SessionRoot, "old"), filepath.Join(o.SessionRoot, "active")
	oldWorkspace, oldEvents := filepath.Join(oldDir, "workspace.yaml"), filepath.Join(oldDir, "events.jsonl")
	activeWorkspace, activeEvents := filepath.Join(activeDir, "workspace.yaml"), filepath.Join(activeDir, "events.jsonl")
	write(t, oldWorkspace, "summary: Old session\n")
	write(t, oldEvents, eventJSON("old", "user.message", map[string]any{"content": "old history"}))
	write(t, activeWorkspace, "summary: Active session\n")
	write(t, activeEvents, eventJSON("first", "user.message", map[string]any{"content": "active history"}))
	oldTime := time.Now().Add(-365 * 24 * time.Hour)
	for _, path := range []string{oldWorkspace, oldEvents} {
		if err := os.Chtimes(path, oldTime, oldTime); err != nil {
			t.Fatal(err)
		}
	}
	opened := watchSourceOpens(t, oldDir, activeDir)
	s := service(t, o)
	s.Scan(context.Background())
	if got, want := opened(), (map[string]int{oldWorkspace: 1, oldEvents: 1, activeWorkspace: 1, activeEvents: 1}); !reflect.DeepEqual(got, want) {
		t.Fatalf("initial opens: %v, want %v", got, want)
	}
	for range 3 {
		s.Scan(context.Background())
	}
	if got := opened(); len(got) != 0 {
		t.Fatalf("unchanged source files reopened: %v", got)
	}

	// Only the growing event file opens; both workspaces and the old history
	// stay closed. Discard the write operation's own open notification first.
	f, err := os.OpenFile(activeEvents, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(eventJSON("second", "assistant.message", map[string]any{"content": "new activity"})); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	opened()
	s.Scan(context.Background())
	if got, want := opened(), (map[string]int{activeEvents: 1}); !reflect.DeepEqual(got, want) {
		t.Fatalf("append opens: %v, want %v", got, want)
	}

	// An old session can become active again; age never excludes it.
	write(t, oldWorkspace, "summary: Reactivated session\n")
	opened()
	s.Scan(context.Background())
	if got, want := opened(), (map[string]int{oldWorkspace: 1}); !reflect.DeepEqual(got, want) {
		t.Fatalf("reactivated opens: %v, want %v", got, want)
	}
	detail, err := s.Detail(context.Background(), "old")
	if err != nil || detail.Session.Name != "Reactivated session" {
		t.Fatalf("old session change missed: %+v %v", detail, err)
	}
	detail, err = s.Detail(context.Background(), "active")
	if err != nil || len(detail.Events) != 2 {
		t.Fatalf("new activity missed: %+v %v", detail, err)
	}

	// Malformed metadata is reported once and stays closed until corrected.
	write(t, oldWorkspace, "summary: first\nsummary: duplicate\n")
	opened()
	s.Scan(context.Background())
	if got, want := opened(), (map[string]int{oldWorkspace: 1}); !reflect.DeepEqual(got, want) {
		t.Fatalf("malformed workspace opens: %v, want %v", got, want)
	}
	s.Scan(context.Background())
	if got := opened(); len(got) != 0 {
		t.Fatalf("unchanged malformed workspace reopened: %v", got)
	}
	write(t, oldWorkspace, "summary: Corrected workspace\n")
	opened()
	s.Scan(context.Background())
	detail, err = s.Detail(context.Background(), "old")
	if err != nil || detail.Session.Name != "Corrected workspace" {
		t.Fatalf("corrected workspace missed: %+v %v", detail, err)
	}
}
