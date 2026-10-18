package mattermost

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/zalando/go-keyring"
)

// TokenStore never falls back to plaintext; unavailable keyrings leave the
// session in memory. Implementations must never store passwords or TOTP codes.
type TokenStore interface {
	Load(context.Context) (string, error)
	Save(context.Context, string) error
	Delete(context.Context) error
}

// Keychain supports macOS Keychain, Windows Credential Manager and Linux Secret
// Service through go-keyring. The service key isolates both server and account.
type Keychain struct{ Server, Account string }

func (k Keychain) key() string {
	return fmt.Sprintf("custom-visning-mattermost-%x", sha256.Sum256([]byte(k.Server+"\x00"+k.Account)))
}
func (k Keychain) Load(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return keyring.Get(k.key(), k.Account)
}
func (k Keychain) Save(ctx context.Context, token string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return keyring.Set(k.key(), k.Account, token)
}
func (k Keychain) Delete(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return keyring.Delete(k.key(), k.Account)
}
