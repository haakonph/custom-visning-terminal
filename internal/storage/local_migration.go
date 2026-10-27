package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/storage/dbgen"
	"github.com/haakonph/custom-visning-terminal/internal/tasks"
)

// LoadLocal imports each legacy local domain once into the shared database.
// Originals remain intact. A changed source is reported instead of silently
// ignoring edits made by an older binary after migration.
func LoadLocal(path, source string, out any) error {
	kind, _, err := domain(out)
	if err != nil {
		return err
	}
	if kind != "tasks" && kind != "entities" && kind != "snoozes" {
		return errors.New("ikke et lokalt domene")
	}
	destination, err := filepath.Abs(Path(path))
	if err != nil {
		return err
	}
	origin, err := filepath.Abs(Path(source))
	if err != nil {
		return err
	}
	if path == "" || source == "" {
		return errors.New("lokal lagringssti mangler")
	}
	if destination == origin {
		if err = Load(path, out); err != nil {
			return err
		}
		if err = Migrate(path, out); err != nil {
			return err
		}
		return Load(path, out)
	}
	// Reading and validating the source is complete before opening a target write.
	sourceValue := newLocalValue(kind)
	loadErr := Load(source, sourceValue)
	if loadErr != nil && !errors.Is(loadErr, os.ErrNotExist) {
		return fmt.Errorf("leser gammelt %s-lager: %w", kind, loadErr)
	}
	if loadErr == nil {
		if err = Migrate(source, sourceValue); err != nil {
			return err
		}
		if err = Load(source, sourceValue); err != nil {
			return err
		}
	}
	var sent map[string]time.Time
	if kind == "entities" {
		if _, statErr := os.Stat(Path(source)); statErr == nil {
			sent, err = LoadReviewMessages(source)
			if err != nil {
				return err
			}
		}
	}
	encoded, err := json.Marshal(struct {
		Data any
		Sent map[string]time.Time
	}{sourceValue, sent})
	if err != nil {
		return err
	}
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(encoded))
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
	ctx := context.Background()
	q := dbgen.New(tx)
	var oldPath, oldFingerprint string
	err = tx.QueryRow("SELECT source_path,fingerprint FROM local_imports WHERE kind=?", kind).Scan(&oldPath, &oldFingerprint)
	if err == nil {
		if oldPath != origin {
			return fmt.Errorf("%s er allerede importert fra %s; flytting krever eksplisitt migrering", kind, oldPath)
		}
		if (loadErr == nil || len(sent) > 0) && oldFingerprint != fingerprint {
			return fmt.Errorf("gammelt %s-lager er endret etter migrering; bevar begge filer og avklar endringene", kind)
		}
	} else {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, stateErr := q.GetState(ctx, kind); stateErr == nil {
			// Existing combined stores are authoritative; never replace their contents.
			if loadErr == nil || len(sent) > 0 {
				return fmt.Errorf("%s finnes i både lokalt og gammelt lager; avklar importen", kind)
			}
			if _, err = tx.Exec("INSERT INTO local_imports(kind,source_path,fingerprint) VALUES(?,?,?)", kind, origin, fingerprint); err != nil {
				return err
			}
			if err = readDomain(ctx, q, out); err != nil {
				return err
			}
			return tx.Commit()
		} else if !errors.Is(stateErr, sql.ErrNoRows) {
			return stateErr
		}
		if kind == "tasks" {
			items := sourceValue.(*[]model.Task)
			if err = tasks.EnsureIDs(*items); err != nil {
				return err
			}
		}
		if kind == "snoozes" {
			for id, e := range *sourceValue.(*map[string]model.SnoozeEntry) {
				if id == "" || id != e.ID || e.Until.IsZero() || e.Until.Year() > 9999 {
					return errors.New("ugyldig gammel utsettelse")
				}
			}
		}
		_, value, domainErr := domain(sourceValue)
		if domainErr != nil {
			return domainErr
		}
		if err = writeDomain(ctx, q, value); err != nil {
			return err
		}
		for link, at := range sent {
			if _, err = tx.Exec(`INSERT INTO review_messages(pull_request_url,sent_at) VALUES(?,?) ON CONFLICT(pull_request_url) DO UPDATE SET sent_at=excluded.sent_at WHERE excluded.sent_at > sent_at`, link, at.UTC().Format("2006-01-02T15:04:05.000000000Z")); err != nil {
				return err
			}
		}
		if err = q.SetState(ctx, kind); err != nil {
			return err
		}
		if _, err = tx.Exec("INSERT INTO local_imports(kind,source_path,fingerprint) VALUES(?,?,?)", kind, origin, fingerprint); err != nil {
			return err
		}
	}
	if err = readDomain(ctx, q, out); err != nil {
		return err
	}
	return tx.Commit()
}
func newLocalValue(kind string) any {
	switch kind {
	case "tasks":
		return new([]model.Task)
	case "entities":
		return new(model.EntityState)
	default:
		return new(map[string]model.SnoozeEntry)
	}
}
