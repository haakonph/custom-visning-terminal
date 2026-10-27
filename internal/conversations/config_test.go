package conversations

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haakonph/custom-visning-terminal/internal/config"
	"github.com/zalando/go-keyring"
)

func TestMattermostConfigNoSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("CUSTOM_VISNING_CONFIG", path)
	t.Setenv("MATTERMOST_SERVER", "https://chat.example/")
	t.Setenv("MATTERMOST_USERNAME", "alice")
	t.Setenv("MATTERMOST_TOKEN", "secret-token")
	if err := config.Update(path, map[string]any{"unrelated": true}); err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if err = SaveConfig(c); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MATTERMOST_SERVER", "")
	t.Setenv("MATTERMOST_USERNAME", "")
	loaded, err := LoadConfig()
	if err != nil || loaded != c || c.Server != "https://chat.example" {
		t.Fatal(loaded, err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret-token") || !strings.Contains(string(raw), "unrelated") {
		t.Fatal("settings leaked secrets or lost unrelated values")
	}
}
func TestKeychainAccountIsolation(t *testing.T) {
	keyring.MockInit()
	ctx := context.Background()
	k := Keychain{Server: "https://chat.example", Account: "alice"}
	if err := k.Save(ctx, "session-token"); err != nil {
		t.Fatal(err)
	}
	token, err := k.Load(ctx)
	if err != nil || token != "session-token" {
		t.Fatal("session not restored", err)
	}
	for _, other := range []Keychain{{Server: k.Server, Account: "bob"}, {Server: "https://other.example", Account: k.Account}} {
		if _, err = other.Load(ctx); err == nil {
			t.Fatal("session shared between accounts")
		}
	}
	if err = k.Delete(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = k.Load(ctx); err == nil {
		t.Fatal("session not deleted")
	}
}
