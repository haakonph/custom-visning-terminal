package conversations

import (
	"context"
	"errors"
	"strings"

	"github.com/haakonph/custom-visning-terminal/internal/model"
	mm "github.com/mattermost/mattermost/server/public/model"
)

type ReviewReactor interface {
	ReactToReview(context.Context, string, string, string) error
}

func (a *API) ReactToReview(ctx context.Context, user, post, emoji string) error {
	_, _, err := a.sdk.SaveReaction(ctx, &mm.Reaction{UserId: user, PostId: post, EmojiName: emoji})
	return apiError(err)
}

// CompleteReview rechecks the current message and channel before writing.
// Mattermost identifies a reaction by user/post/emoji, so repeating it is safe.
func (s *Service) CompleteReview(ctx context.Context, postID, prURL string, review model.ReviewOutcome) error {
	var emoji string
	switch review {
	case model.ReviewApproval:
		emoji = "white_check_mark"
	case model.ReviewComment:
		emoji = "speech_balloon"
	default:
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	reactor, ok := s.API.(ReviewReactor)
	if !ok {
		return errors.New("Mattermost reactions unavailable")
	}
	for _, post := range s.Data.Posts {
		if post.ID != postID || post.DeletedAt != 0 {
			continue
		}
		selected := false
		for _, channel := range s.Data.Channels {
			if channel.ID == post.ChannelID && channel.Selected {
				selected = true
				break
			}
		}
		if !selected {
			return nil
		}
		for _, link := range PRLinks(post.Message) {
			if !strings.EqualFold(link.URL, prURL) {
				continue
			}
			user, err := s.API.Me(ctx)
			if err != nil {
				return err
			}
			if user == "" {
				return ErrSession
			}
			return reactor.ReactToReview(ctx, user, post.ID, emoji)
		}
	}
	return nil
}
