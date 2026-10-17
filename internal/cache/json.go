// Package cache provides a private, account-scoped, atomically replaced JSON cache.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/haakonph/custom-visning-terminal/internal/model"
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
	f, err := os.Open(s.path(user))
	if errors.Is(err, os.ErrNotExist) {
		return model.Snapshot{Version: 1, User: user}, nil
	}
	if err != nil {
		return snap, err
	}
	defer f.Close()
	dec := json.NewDecoder(io.LimitReader(f, 32<<20))
	if err = dec.Decode(&snap); err != nil {
		return model.Snapshot{}, fmt.Errorf("invalid cache: %w", err)
	}
	var extra any
	if err = dec.Decode(&extra); err != io.EOF {
		return model.Snapshot{}, errors.New("invalid trailing cache data")
	}
	if snap.Version != 1 || snap.User == "" || (user != "" && !strings.EqualFold(snap.User, user)) {
		return model.Snapshot{}, errors.New("unsupported cache version or account")
	}
	return snap, nil
}
func (s *Store) Save(snap model.Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if snap.User == "" {
		return errors.New("cache requires an account")
	}
	if err := os.MkdirAll(s.Dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(s.Dir, ".snapshot-*.json")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = json.NewEncoder(f).Encode(snap); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, s.path(snap.User))
}
