package conversations

import (
	"context"
	"errors"
	"os"

	"github.com/zalando/go-keyring"
)

// Webhooks use a separate keychain entry from Mattermost login sessions.
func webhookKeychain() Keychain {
	return Keychain{Server: "incoming-webhook", Account: "default"}
}

// SaveWebhook validates and persists the URL without writing it to config files.
func SaveWebhook(ctx context.Context, endpoint string) error {
	if _, err := New(endpoint); err != nil {
		return err
	}
	if err := webhookKeychain().Save(ctx, endpoint); err != nil {
		return errors.New("kunne ikke lagre webhooken i operativsystemets nøkkelring; lås opp eller aktiver nøkkelringen og prøv igjen")
	}
	return nil
}

// LoadWebhook gives an explicit environment override precedence over the keychain.
func LoadWebhook(ctx context.Context) (string, error) {
	if endpoint := os.Getenv("MATTERMOST_WEBHOOK_URL"); endpoint != "" {
		return endpoint, nil
	}
	endpoint, err := webhookKeychain().Load(ctx)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", errors.New("kunne ikke lese lagret webhook fra operativsystemets nøkkelring")
	}
	return endpoint, nil
}
