package mattermost

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestWebhookRestoredAfterRestart(t *testing.T) {
	keyring.MockInit()
	t.Setenv("MATTERMOST_WEBHOOK_URL", "")
	ctx := context.Background()
	if endpoint, err := LoadWebhook(ctx); err != nil || endpoint != "" {
		t.Fatal("expected empty initial setup")
	}
	const saved = "https://chat.example/hooks/private-token"
	if err := SaveWebhook(ctx, saved); err != nil {
		t.Fatal(err)
	}
	endpoint, err := LoadWebhook(context.Background())
	if err != nil || endpoint != saved {
		t.Fatal("saved webhook not restored")
	}
	if _, err := New(endpoint); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MATTERMOST_WEBHOOK_URL", "https://other.example/hooks/override")
	if endpoint, _ := LoadWebhook(ctx); endpoint != "https://other.example/hooks/override" {
		t.Fatal("environment override ignored")
	}
	t.Setenv("MATTERMOST_WEBHOOK_URL", "")
	if endpoint, _ := LoadWebhook(ctx); endpoint != saved {
		t.Fatal("override replaced saved webhook")
	}
	if err := SaveWebhook(ctx, "http://invalid.example"); err == nil {
		t.Fatal("invalid URL saved")
	}
	if endpoint, _ := LoadWebhook(ctx); endpoint != saved {
		t.Fatal("invalid URL replaced saved webhook")
	}
}

func TestWebhookKeychainFailureDoesNotExposeSecret(t *testing.T) {
	keyring.MockInitWithError(errors.New("keyring failure private-token"))
	t.Cleanup(func() { keyring.MockInit() })
	t.Setenv("MATTERMOST_WEBHOOK_URL", "")
	ctx := context.Background()
	for _, err := range []error{SaveWebhook(ctx, "https://chat.example/hooks/private-token"), func() error { _, err := LoadWebhook(ctx); return err }()} {
		if err == nil || strings.Contains(err.Error(), "private-token") {
			t.Fatal("missing or unsafe keyring error")
		}
	}
	t.Setenv("MATTERMOST_WEBHOOK_URL", "https://chat.example/hooks/environment")
	if _, err := LoadWebhook(ctx); err != nil {
		t.Fatal("environment override requires keyring")
	}
}
