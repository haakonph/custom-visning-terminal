// Package storage persists provider and local data in structured SQLite tables.
// Configuration and upstream import files remain outside the database.
package storage

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/storage/dbgen"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Path preserves the storage locations and overrides used before normalization.
func Path(path string) string {
	if strings.HasSuffix(path, ".json") {
		return strings.TrimSuffix(path, ".json") + ".sqlite"
	}
	if path == "" || strings.HasSuffix(path, ".sqlite") || strings.HasSuffix(path, ".sqlite3") || strings.HasSuffix(path, ".db") {
		return path
	}
	return path + ".sqlite"
}

func open(path string) (*sql.DB, error) {
	if path == "" {
		return nil, errors.New("storage path unavailable")
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return nil, errors.New("storage path is a directory")
	}
	path = Path(path)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}
	db, err := sql.Open("sqlite", u.String()+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_pragma=secure_delete(ON)&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err = applyMigrations(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func applyMigrations(db *sql.DB) error {
	return applyMigrationsFrom(db, migrations)
}

func applyMigrationsFrom(db *sql.DB, source fs.FS) error {
	bootstrap, err := fs.ReadFile(source, "migrations/000_versions.sql")
	if err != nil {
		return err
	}
	if _, err = db.Exec(string(bootstrap)); err != nil {
		return err
	}
	entries, err := fs.ReadDir(source, "migrations")
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current int
	if err = tx.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&current); err != nil {
		return err
	}
	latest := len(entries) - 1
	if current > latest {
		return fmt.Errorf("database schema version %d is newer than supported version %d", current, latest)
	}
	for _, entry := range entries {
		version, err := strconv.Atoi(strings.SplitN(entry.Name(), "_", 2)[0])
		if err != nil {
			return err
		}
		if version <= current {
			continue
		}
		script, err := fs.ReadFile(source, "migrations/"+entry.Name())
		if err != nil {
			return err
		}
		if _, err = tx.Exec(string(script)); err != nil {
			return fmt.Errorf("schema migration %s: %w", entry.Name(), err)
		}
		if _, err = tx.Exec("INSERT INTO schema_migrations(version) VALUES (?)", version); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func decode(data []byte, value any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(value); err != nil {
		return err
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return errors.New("invalid trailing storage data")
	}
	return nil
}

func domain(value any) (string, any, error) {
	v := reflect.ValueOf(value)
	if !v.IsValid() {
		return "", nil, errors.New("missing storage value")
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return "", nil, errors.New("nil storage value")
		}
		value = v.Elem().Interface()
	}
	kind := ""
	switch value.(type) {
	case model.Snapshot:
		kind = "github"
	case model.JiraCache:
		kind = "jira"
	case model.CopilotStore:
		kind = "copilot"
	case []model.Task:
		kind = "tasks"
	case model.EntityState:
		kind = "entities"
	case map[string]string:
		kind = "repositories"
	case map[string]model.LocalStatus:
		kind = "worktrees"
	case map[string]model.SnoozeEntry:
		kind = "snoozes"
	default:
		return "", nil, fmt.Errorf("unsupported storage type %T", value)
	}
	return kind, value, nil
}

// Load reads a consistent normalized snapshot, or legacy data for domain
// validation. Migrate must be called after validation to commit a legacy import.
func Load(path string, value any) error {
	kind, _, err := domain(value)
	if err != nil {
		return err
	}
	if reflect.ValueOf(value).Kind() != reflect.Pointer {
		return errors.New("load requires a destination pointer")
	}
	if path == "" {
		return errors.New("storage path unavailable")
	}
	if _, err := os.Stat(Path(path)); err == nil {
		db, err := open(path)
		if err != nil {
			return err
		}
		defer db.Close()
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		q := dbgen.New(tx)
		ctx := context.Background()
		_, err = q.GetState(ctx, kind)
		if err == nil {
			return readDomain(ctx, q, value)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		data, err := q.LoadLegacyDocument(ctx)
		if err == nil {
			return decode(data, value)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if Path(path) == path {
		return os.ErrNotExist
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err = decode(data, value); err != nil {
		return fmt.Errorf("invalid legacy storage: %w", err)
	}
	return nil
}

// Save replaces a domain snapshot in one transaction, including its relations.
func Save(path string, value any) error { return write(path, value, false) }

// Migrate imports a validated legacy value once. It cannot replace a snapshot
// already committed by another process. Legacy JSON files are left untouched.
func Migrate(path string, value any) error { return write(path, value, true) }

func write(path string, value any, onlyMissing bool) error {
	kind, value, err := domain(value)
	if err != nil {
		return err
	}
	db, err := open(path)
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := dbgen.New(tx)
	ctx := context.Background()
	if onlyMissing {
		_, err = q.GetState(ctx, kind)
		if err == nil {
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	if err = writeDomain(ctx, q, value); err != nil {
		return err
	}
	if err = q.SetState(ctx, kind); err != nil {
		return err
	}
	if err = q.DeleteLegacyDocument(ctx); err != nil {
		return err
	}
	return tx.Commit()
}
