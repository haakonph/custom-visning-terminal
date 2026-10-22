package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/storage"
	"github.com/haakonph/custom-visning-terminal/internal/usage"
)

func measuredModel(t *testing.T) (Model, *usage.Recorder) {
	t.Helper()
	r, err := usage.Open(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	return ready(t).WithUsage(r, ""), r
}

func metricCount(t *testing.T, r *usage.Recorder, category, name string) int64 {
	t.Helper()
	counts, err := r.Snapshot(0)
	if err != nil {
		t.Fatal(err)
	}
	var n int64
	for _, c := range counts {
		if c.Category == category && c.Name == name {
			n += c.Count
		}
	}
	return n
}

func TestUsageCountsRealNavigationAndSingleInternalDispatch(t *testing.T) {
	m, r := measuredModel(t)
	m, _ = sequence(m, "jjld") // second j reaches the boundary; drafts are empty.
	if got := metricCount(t, r, "navigation", "row"); got != 1 {
		t.Fatal("counted a boundary as movement", got)
	}
	if got := metricCount(t, r, "navigation", "tab"); got != 1 {
		t.Fatal("tab change", got)
	}
	if got := metricCount(t, r, "command", "details"); got != 1 {
		t.Fatal("internal dispatch counted twice", got)
	}
	m, _ = sequence(m, "vo")
	if got := metricCount(t, r, "command", "view"); got != 1 {
		t.Fatal("sequence counted before completion", got)
	}
	if got := metricCount(t, r, "navigation", "screen"); got != 1 {
		t.Fatal("provider change missing", got)
	}
}

func TestUsageNeverRecordsTypedContentOrBackgroundActivity(t *testing.T) {
	m, r := measuredModel(t)
	m, _ = sequence(m, "/")
	before, _ := r.Snapshot(0)
	m, _ = sequence(m, "private-search-atfq-jkl-vm")
	m, _ = update(m, tea.PasteMsg{Content: "secret pasted text"})
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = update(m, tea.WindowSizeMsg{Width: 120, Height: 30})
	m, _ = update(m, snoozeTick{})
	after, _ := r.Snapshot(0)
	if !sameUsage(before, after) {
		t.Fatalf("text input or background activity counted: before=%+v after=%+v", before, after)
	}
	m.settings.active, m.settings.editing = true, true
	m, _ = sequence(m, "secret-token-atfq-jkl")
	counts, _ := r.Snapshot(0)
	if !sameUsage(after, counts) {
		t.Fatal("settings text counted", counts)
	}
}

func sameUsage(a, b []storage.UsageCount) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestUsageViewPeriodsResizeAndReturn(t *testing.T) {
	m, r := measuredModel(t)
	m, _ = sequence(m, "vm")
	if !m.usageView || !strings.Contains(ansi.Strip(m.View().Content), "BRUKSSTATISTIKK") {
		t.Fatal("metrics view missing")
	}
	m, _ = sequence(m, "l")
	if !strings.Contains(ansi.Strip(m.View().Content), "30 dager") {
		t.Fatal("period did not change")
	}
	before, _ := r.Snapshot(0)
	m.height = 8
	m, _ = sequence(m, strings.Repeat("j", 500))
	if !strings.Contains(ansi.Strip(m.View().Content), "Åpne visning") {
		t.Fatal("scrolled past the final row")
	}
	lastOffset := m.usageOffset
	m, _ = sequence(m, "k")
	if m.usageOffset != max(0, lastOffset-1) || lastOffset != m.usageMaxOffset() {
		t.Fatal("could not scroll back immediately after reaching the end")
	}
	m, _ = update(m, tea.WindowSizeMsg{Width: 20, Height: 2})
	if len(strings.Split(m.View().Content, "\n")) > 2 {
		t.Fatal("view overflows small terminal")
	}
	after, _ := r.Snapshot(0)
	if !sameUsage(before, after) {
		t.Fatal("metrics navigation polluted usage", after)
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.usageView || m.visibleProvider() != "github" {
		t.Fatal("did not return to original view")
	}
}

func TestUsageViewWithoutRecorder(t *testing.T) {
	m := ready(t).WithUsage(nil, "Deaktivert")
	m, _ = sequence(m, "vm")
	if !strings.Contains(m.View().Content, "Deaktivert") {
		t.Fatal("disabled state missing")
	}
}

func TestUsageViewShowsIncomingRequests(t *testing.T) {
	m, r := measuredModel(t)
	r.Record("http.incoming", "POST", "/v1/logs", "200")
	r.Record("http.incoming", "GET", "unknown", "404")
	m, _ = sequence(m, "vm")
	view := ansi.Strip(m.View().Content)
	for _, text := range []string{"Innkommende HTTP-kall", "POST", "/v1/logs → 200", "GET", "Ukjent endepunkt → 404"} {
		if !strings.Contains(view, text) {
			t.Errorf("metrics view missing %q: %s", text, view)
		}
	}
}

func TestUsageTracksTaskAndChatNavigationWithoutEntityNames(t *testing.T) {
	m, r := measuredModel(t)
	m.tasks.items = []model.Task{{ID: "private-id-1", Title: "private-title-1"}, {ID: "private-id-2", Title: "private-title-2"}}
	m, _ = sequence(m, "vol")
	if metricCount(t, r, "navigation", "item") != 1 {
		t.Fatal("task navigation missing")
	}
	chat := readerModel(t).WithUsage(r, "")
	chat, _ = sequence(chat, "d")
	counts, err := r.Snapshot(0)
	if err != nil {
		t.Fatal(err)
	}
	foundThread := false
	for _, c := range counts {
		if c.Target == "mattermost.thread" {
			foundThread = true
		}
		if strings.Contains(c.Source+c.Target+c.Name, "private") {
			t.Fatal("entity metadata in labels", c)
		}
	}
	if !foundThread {
		t.Fatal("chat thread navigation missing", counts)
	}
	before, _ := r.Snapshot(0)
	chat.mmReader.root = ""
	chat.mmReader.mode = "Password"
	chat, _ = sequence(chat, "private-password-jkl-atfq")
	after, _ := r.Snapshot(0)
	if !sameUsage(before, after) {
		t.Fatal("chat password input counted", after)
	}
}
