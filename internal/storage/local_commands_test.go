package storage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/haakonph/custom-visning-terminal/internal/localstate"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/relations"
	"github.com/haakonph/custom-visning-terminal/internal/tasks"
)

func createTask(t *testing.T, path, id string) model.Task {
	t.Helper()
	rows, err := tasks.Create(context.Background(), TaskStore{path}, model.Task{ID: id, Title: id})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.ID == id {
			return r
		}
	}
	t.Fatal("missing task")
	return model.Task{}
}
func TestIndependentTaskChangesPreserveOtherWriters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "local.sqlite")
	original := createTask(t, path, "original")
	a := []model.Task{original, {ID: "a", Title: "A"}}
	b := []model.Task{original, {ID: "b", Title: "B"}}
	changesA, _ := tasks.Changes([]model.Task{original}, a)
	changesB, _ := tasks.Changes([]model.Task{original}, b)
	if _, err := (TaskStore{path}).ApplyTaskChanges(context.Background(), changesA); err != nil {
		t.Fatal(err)
	}
	rows, err := (TaskStore{path}).ApplyTaskChanges(context.Background(), changesB)
	if err != nil || len(rows) != 3 {
		t.Fatal(rows, err)
	}
}
func TestTaskEditAndDeleteConflictsPreserveUserData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "local.sqlite")
	original := createTask(t, path, "original")
	edited := original
	edited.Title = "A's edit"
	if _, err := tasks.Update(context.Background(), TaskStore{path}, original, edited); err != nil {
		t.Fatal(err)
	}
	other := original
	other.Title = "B's draft"
	if _, err := tasks.Update(context.Background(), TaskStore{path}, original, other); !errors.Is(err, tasks.ErrConflict) {
		t.Fatal("edit conflict not reported", err)
	}
	if _, err := tasks.Delete(context.Background(), TaskStore{path}, original); !errors.Is(err, tasks.ErrConflict) {
		t.Fatal("delete conflict not reported", err)
	}
	var actual []model.Task
	if err := Load(path, &actual); err != nil || actual[0].Title != "A's edit" {
		t.Fatal(actual, err)
	}
}
func TestLocalCommandRollsBackAcrossDomains(t *testing.T) {
	path := filepath.Join(t.TempDir(), "local.sqlite")
	original := createTask(t, path, "original")
	edited := original
	edited.Title = "edited"
	ref := model.TaskReference{Provider: "github", URL: "https://github.com/o/r/pull/1"}
	changes := localstate.Changes{Tasks: []tasks.Change{{Before: &original, After: &edited}}, Relations: relations.Changes{Archive: []relations.ArchiveChange{{After: &ref}}}}
	db, err := open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TRIGGER fail_archive BEFORE INSERT ON archived_entities BEGIN SELECT RAISE(ABORT,'injected disk failure'); END`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := (LocalStore{path}).CommitLocal(context.Background(), changes); err == nil {
		t.Fatal("injected failure did not fire")
	}
	var rows []model.Task
	if err = Load(path, &rows); err != nil || rows[0].Title != original.Title || rows[0].Revision != original.Revision {
		t.Fatal("task partially committed", rows, err)
	}
	db, err = open(path)
	if err != nil {
		t.Fatal(err)
	}
	db.Exec("DROP TRIGGER fail_archive")
	db.Close()
	// A failure after both task and archive writes must also roll everything back.
	bad := model.SnoozeEntry{ID: "bad", Kind: "github"}
	changes.Snoozes = []localstate.SnoozeChange{{After: &bad}}
	if _, err = (LocalStore{path}).CommitLocal(context.Background(), changes); err == nil {
		t.Fatal("invalid snooze accepted")
	}
	db, err = open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err = db.QueryRow("SELECT COUNT(*) FROM archived_entities").Scan(&count); err != nil || count != 0 {
		t.Fatal("archive partially committed", count, err)
	}
}
func TestLegacyLocalStoresImportOnceAndDetectOldWriters(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "local.sqlite")
	source := filepath.Join(dir, "links.sqlite")
	state := model.EntityState{Archived: []model.TaskReference{{Provider: "jira", URL: "https://jira.test/browse/A-1"}}}
	if err := Save(source, state); err != nil {
		t.Fatal(err)
	}
	var loaded model.EntityState
	if err := LoadLocal(path, source, &loaded); err != nil || len(loaded.Archived) != 1 {
		t.Fatal(loaded, err)
	}
	next := model.TaskReference{Provider: "github", URL: "https://github.com/o/r/pull/2"}
	if _, err := (LocalStore{path}).CommitLocal(context.Background(), localstate.Changes{Relations: relations.Changes{Archive: []relations.ArchiveChange{{After: &next}}}}); err != nil {
		t.Fatal(err)
	}
	if err := LoadLocal(path, source, &loaded); err != nil || len(loaded.Archived) != 2 {
		t.Fatal("reimport replaced local changes", loaded, err)
	}
	state.Archived = nil
	if err := Save(source, state); err != nil {
		t.Fatal(err)
	}
	if err := LoadLocal(path, source, &loaded); err == nil {
		t.Fatal("old writer silently ignored")
	}
}
func TestTaskProcessWriter(t *testing.T) {
	path := os.Getenv("CUSTOM_VISNING_TEST_TASK_DB")
	if path == "" {
		return
	}
	data, err := os.ReadFile(os.Getenv("CUSTOM_VISNING_TEST_TASK_BASE"))
	if err != nil {
		t.Fatal(err)
	}
	var baseline []model.Task
	if err = json.Unmarshal(data, &baseline); err != nil {
		t.Fatal(err)
	}
	id := os.Getenv("CUSTOM_VISNING_TEST_TASK_ID")
	after := append(tasks.Clone(baseline), model.Task{ID: id, Title: id})
	changes, err := tasks.Changes(baseline, after)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = (TaskStore{path}).ApplyTaskChanges(context.Background(), changes); err != nil {
		t.Fatal(err)
	}
}
func TestSeparateProcessesCannotOverwriteEachOthersTasks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "local.sqlite")
	original := createTask(t, path, "original")
	baseline := filepath.Join(dir, "baseline.json")
	data, _ := json.Marshal([]model.Task{original})
	if err := os.WriteFile(baseline, data, 0600); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	commands := []*exec.Cmd{}
	for _, id := range []string{"process-a", "process-b"} {
		cmd := exec.CommandContext(ctx, executable, "-test.run=^TestTaskProcessWriter$")
		cmd.Env = append(os.Environ(), "CUSTOM_VISNING_TEST_TASK_DB="+path, "CUSTOM_VISNING_TEST_TASK_BASE="+baseline, "CUSTOM_VISNING_TEST_TASK_ID="+id)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		commands = append(commands, cmd)
	}
	for _, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	var actual []model.Task
	if err := Load(path, &actual); err != nil || len(actual) != 3 {
		t.Fatal(actual, err)
	}
}

func TestImportPreservesExistingLocalDomainWhenOldSourceIsMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "local.sqlite")
	ref := model.TaskReference{Provider: "jira", URL: "https://jira.test/browse/A-1"}
	if _, err := (LocalStore{path}).CommitLocal(context.Background(), localstate.Changes{Relations: relations.Changes{Archive: []relations.ArchiveChange{{After: &ref}}}}); err != nil {
		t.Fatal(err)
	}
	var loaded model.EntityState
	if err := LoadLocal(path, filepath.Join(dir, "absent.sqlite"), &loaded); err != nil || len(loaded.Archived) != 1 {
		t.Fatal("missing source erased local domain", loaded, err)
	}
}
func TestImportPreservesReviewSendTimesAndRejectsNewerSchemas(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "local.sqlite")
	source := filepath.Join(dir, "links.sqlite")
	at := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	link := "https://github.com/o/r/pull/1"
	if err := SaveReviewMessages(source, []string{link}, at); err != nil {
		t.Fatal(err)
	}
	var loaded model.EntityState
	if err := LoadLocal(path, source, &loaded); err != nil {
		t.Fatal(err)
	}
	times, err := LoadReviewMessages(path)
	if err != nil || !times[link].Equal(at) {
		t.Fatal(times, err)
	}
	if err := LoadLocal(path, source, &loaded); err != nil {
		t.Fatal("repeat import failed", err)
	}
	db, err := open(source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO schema_migrations(version) VALUES(999)"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := LoadLocal(path, source, &loaded); err == nil {
		t.Fatal("unsupported source schema silently accepted")
	}
}

func TestUntouchedDomainCanStillImportAfterAnotherLocalCommand(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "local.sqlite")
	source := filepath.Join(dir, "tasks.sqlite")
	ref := model.TaskReference{Provider: "github", URL: "https://github.com/o/r/pull/1"}
	if _, err := (LocalStore{path}).CommitLocal(context.Background(), localstate.Changes{Relations: relations.Changes{Archive: []relations.ArchiveChange{{After: &ref}}}}); err != nil {
		t.Fatal(err)
	}
	if err := Save(source, []model.Task{{ID: "recovered", Title: "Recovered task"}}); err != nil {
		t.Fatal(err)
	}
	var loaded []model.Task
	if err := LoadLocal(path, source, &loaded); err != nil || len(loaded) != 1 || loaded[0].ID != "recovered" {
		t.Fatal("unrelated command initialized an empty task store", loaded, err)
	}
}
