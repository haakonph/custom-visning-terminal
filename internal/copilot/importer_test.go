package copilot

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenRegularSkipsUnchangedAndDetectsChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source")
	write(t, path, "original")
	f, original, err := openRegular(path, nil)
	if err != nil || f == nil {
		t.Fatalf("first open: %v", err)
	}
	f.Close()
	f, _, err = openRegular(path, original)
	if err != nil || f != nil {
		if f != nil {
			f.Close()
		}
		t.Fatalf("unchanged file should be skipped: %v", err)
	}

	// Same length and inode, but a new modification time.
	write(t, path, "modified")
	modifiedAt := original.ModTime().Add(time.Second)
	if err := os.Chtimes(path, modifiedAt, modifiedAt); err != nil {
		t.Fatal(err)
	}
	f, modified, err := openRegular(path, original)
	if err != nil || f == nil {
		t.Fatalf("same-size change skipped: %v", err)
	}
	f.Close()

	// A rotated file with the same length and timestamp is still a new file.
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	write(t, path, "replaced")
	if err := os.Chtimes(path, modified.ModTime(), modified.ModTime()); err != nil {
		t.Fatal(err)
	}
	f, _, err = openRegular(path, modified)
	if err != nil || f == nil {
		t.Fatalf("replacement skipped: %v", err)
	}
	f.Close()

	// Cached metadata must not allow a symlink to bypass regular-file checks.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path+".old", path); err != nil {
		t.Skip(err)
	}
	if f, _, err := openRegular(path, modified); err == nil || f != nil {
		t.Fatal("cached source followed a symlink")
	}
}

func TestMetadataSkipResumesUnreadBytes(t *testing.T) {
	o := testOptions(t)
	path := filepath.Join(o.SessionRoot, "s1", "events.jsonl")
	first := eventJSON("first", "user.message", map[string]any{"content": "first"})
	second := eventJSON("second", "assistant.message", map[string]any{"content": "second"})
	write(t, path, first+second)
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	s := service(t, o)
	// Model cancellation after the first line, with metadata for the entire
	// file already recorded. Unchanged metadata must not hide the second line.
	s.acceptLine(context.Background(), "s1", []byte(first), true)
	s.cursors[path] = &cursor{info: info, position: int64(len(first))}
	s.Scan(context.Background())
	detail, err := s.Detail(context.Background(), "s1")
	if err != nil || len(detail.Events) != 2 || s.cursors[path].position != info.Size() {
		t.Fatalf("unread bytes skipped: %+v %v", detail, err)
	}
}
