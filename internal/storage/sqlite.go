// Package storage persists JSON-encoded domain documents in SQLite transactions.
// Configuration files and upstream import files are deliberately outside this store.
package storage

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// Path maps an old JSON data path to its SQLite replacement. Explicit database
// paths are used as supplied, so environment overrides can select either form.
func Path(path string) string {
	if strings.HasSuffix(path, ".json") {
		return strings.TrimSuffix(path, ".json") + ".sqlite"
	}
	if strings.HasSuffix(path, ".sqlite") || strings.HasSuffix(path, ".sqlite3") || strings.HasSuffix(path, ".db") || path == "" {
		return path
	}
	return path + ".sqlite"
}

func open(path string) (*sql.DB, error) {
	if path == "" {
		return nil, errors.New("storage path unavailable")
	}
	path = Path(path)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	// Create privately before SQLite opens it (including rollback journal files).
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
	db, err := sql.Open("sqlite", u.String()+"?_pragma=busy_timeout(5000)&_pragma=secure_delete(ON)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`CREATE TABLE IF NOT EXISTS documents (id INTEGER PRIMARY KEY CHECK (id = 1), payload BLOB NOT NULL)`); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
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

// Load reads SQLite first, falling back to the old JSON document only before
// migration. It never modifies the legacy file. Missing data returns os.ErrNotExist.
// Call Migrate after domain validation to complete a legacy import.
func Load(path string, value any) error {
	if path == "" {
		return errors.New("storage path unavailable")
	}
	if _, err := os.Stat(Path(path)); err == nil {
		db, err := open(path)
		if err != nil {
			return err
		}
		defer db.Close()
		var data []byte
		err = db.QueryRow(`SELECT payload FROM documents WHERE id = 1`).Scan(&data)
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

// Save replaces one domain document atomically. The transaction is also the
// migration boundary; once committed the old JSON file is never read again.
func Save(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	db, err := open(path)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO documents(id, payload) VALUES(1, ?) ON CONFLICT(id) DO UPDATE SET payload = excluded.payload`, data)
	return err
}

// Migrate commits a validated legacy document without replacing an existing row.
func Migrate(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	db, err := open(path)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO documents(id, payload) VALUES(1, ?) ON CONFLICT(id) DO NOTHING`, data)
	return err
}
