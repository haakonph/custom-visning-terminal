package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestReviewMessagesPersistLatestSend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "links.sqlite")
	at := time.Now().UTC()
	for _, sent := range []time.Time{at, at.Add(time.Hour), at} {
		if err := SaveReviewMessages(path, []string{"pr1", "pr2"}, sent); err != nil {
			t.Fatal(err)
		}
	}
	got, err := LoadReviewMessages(path)
	if err != nil || len(got) != 2 || !got["pr1"].Equal(at.Add(time.Hour)) || !got["pr2"].Equal(at.Add(time.Hour)) {
		t.Fatalf("lost latest send: %v, %v", got, err)
	}
}
