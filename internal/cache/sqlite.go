// Package cache provides a private, account-scoped, SQLite cache.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/storage"
)

type Store struct {
	Dir           string
	credentialKey string
	mu            sync.Mutex
}

func Default(credential string) (*Store, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	// Only a one-way fingerprint is retained, never the credential itself.
	sum := sha256.Sum256([]byte(credential))
	return &Store{Dir: filepath.Join(dir, "custom-visning"), credentialKey: hex.EncodeToString(sum[:])}, nil
}
func (s *Store) path(user string) string {
	if s.credentialKey != "" {
		return filepath.Join(s.Dir, s.credentialKey+".json")
	}
	sum := sha256.Sum256([]byte(strings.ToLower(user)))
	return filepath.Join(s.Dir, hex.EncodeToString(sum[:16])+".json")
}
func (s *Store) Load(user string) (model.Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var snap model.Snapshot
	err := storage.Load(s.path(user), &snap)
	if errors.Is(err, os.ErrNotExist) {
		return model.Snapshot{Version: 1, User: user}, nil
	}
	if err != nil {
		return model.Snapshot{}, fmt.Errorf("invalid cache: %w", err)
	}
	if snap.Version != 1 || snap.User == "" || (user != "" && !strings.EqualFold(snap.User, user)) {
		return model.Snapshot{}, errors.New("unsupported cache version or account")
	}
	if err := storage.Migrate(s.path(user), snap); err != nil {
		return model.Snapshot{}, err
	}
	return snap, nil
}
func (s *Store) Save(snap model.Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if snap.User == "" {
		return errors.New("cache requires an account")
	}
	return storage.Save(s.path(snap.User), snap)
}
