package conversations

import (
	"context"
	"github.com/haakonph/custom-visning-terminal/internal/storage"
	"time"
)

type Sender interface {
	Send(context.Context, string) error
}
type ShareResult struct {
	Err        error
	Sent       int
	Timestamps map[string]time.Time
	SaveErr    error
}

// Share stops at the first failed send. A caller may retry the remaining draft,
// but an ambiguous network outcome is never retried automatically.
func Share(ctx context.Context, sender Sender, messages []string, localPath string) ShareResult {
	result := ShareResult{Timestamps: map[string]time.Time{}}
	for _, text := range messages {
		if err := ctx.Err(); err != nil {
			result.Err = err
			break
		}
		if err := sender.Send(ctx, text); err != nil {
			result.Err = err
			break
		}
		result.Sent++
		at := time.Now()
		var links []string
		for _, link := range PRLinks(text) {
			links = append(links, link.URL)
			result.Timestamps[link.URL] = at
		}
		if len(links) > 0 {
			if err := storage.SaveReviewMessages(localPath, links, at); err != nil {
				result.SaveErr = err
			}
		}
	}
	return result
}
