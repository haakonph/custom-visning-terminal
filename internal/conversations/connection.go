package conversations

import (
	"os"
	"path/filepath"
)

type Connection struct {
	Service *Service
	API     *API
	Tokens  TokenStore
	Account string
}

// Connect prepares an account's local cache. Authentication remains asynchronous.
func Connect(c Config) (Connection, error) {
	var result Connection
	api, err := NewAPI(c.Server)
	if err != nil {
		return result, err
	}
	api.ExpectAccount(c.Username)
	dir, err := os.UserCacheDir()
	if err != nil {
		return result, err
	}
	service, err := OpenService(api, filepath.Join(dir, "custom-visning", "mattermost.sqlite"), c.Server, c.Username)
	if err != nil {
		return result, err
	}
	result = Connection{Service: service, API: api, Tokens: Keychain{Server: c.Server, Account: c.Username}, Account: c.Username}
	return result, nil
}
