package conversations

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestAPIReviewReactionUsesAuthenticatedUserAndEmoji(t *testing.T) {
	for _, emoji := range []string{"white_check_mark", "speech_balloon"} {
		t.Run(emoji, func(t *testing.T) {
			for _, status := range []int{http.StatusOK, http.StatusUnauthorized, http.StatusForbidden} {
				t.Run(http.StatusText(status), func(t *testing.T) {
					srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodPost || r.URL.Path != "/api/v4/reactions" || !strings.EqualFold(r.Header.Get("Authorization"), "Bearer test-token") {
							t.Error(r.Method, r.URL)
						}
						var reaction map[string]any
						if err := json.NewDecoder(r.Body).Decode(&reaction); err != nil {
							t.Error(err)
						}
						if reaction["user_id"] != "me" || reaction["post_id"] != "original" || reaction["emoji_name"] != emoji {
							t.Error(reaction)
						}
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(status)
						if status == http.StatusOK {
							_ = json.NewEncoder(w).Encode(reaction)
						} else {
							_ = json.NewEncoder(w).Encode(map[string]any{"id": "test.error", "status_code": status})
						}
					}))
					defer srv.Close()
					api, _ := NewAPI(srv.URL)
					api.sdk.HTTPClient = srv.Client()
					api.SetToken("test-token")
					err := api.ReactToReview(context.Background(), "me", "original", emoji)
					if status == http.StatusOK && err != nil || status == http.StatusUnauthorized && !errors.Is(err, ErrSession) || status == http.StatusForbidden && !errors.Is(err, ErrAccess) {
						t.Fatal(err)
					}
				})
			}
		})
	}
}

type reactingReader struct {
	fakeReader
	user, post, emoji string
}

func (r *reactingReader) ReactToReview(_ context.Context, user, post, emoji string) error {
	r.user, r.post, r.emoji = user, post, emoji
	return nil
}

func TestCompleteReviewRechecksMessageAndChannel(t *testing.T) {
	for _, scenario := range []string{"valid", "deleted", "deselected", "edited", "missing", "expired"} {
		t.Run(scenario, func(t *testing.T) {
			api := &reactingReader{}
			link := "https://github.com/acme/api/pull/42"
			data := model.MattermostData{Channels: []model.MattermostChannel{{ID: "c", Selected: true}}, Posts: []model.MattermostPost{{ID: "p", ChannelID: "c", Message: link}}}
			switch scenario {
			case "deleted":
				data.Posts[0].DeletedAt = 1
			case "deselected":
				data.Channels[0].Selected = false
			case "edited":
				data.Posts[0].Message = "link removed"
			case "missing":
				data.Posts = nil
			case "expired":
				api.err = ErrSession
			}
			service := &Service{API: api, Data: data}
			err := service.CompleteReview(context.Background(), "p", link, model.ReviewApproval)
			if scenario == "expired" {
				if !errors.Is(err, ErrSession) {
					t.Fatal(err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if scenario == "valid" {
				if api.user != "user" || api.post != "p" || api.emoji != "white_check_mark" {
					t.Fatal(api)
				}
			} else if api.post != "" {
				t.Fatal("reacted to an invalid request", api)
			}
		})
	}
}

func TestCompleteReviewSelectsReaction(t *testing.T) {
	for _, tc := range []struct {
		review model.ReviewOutcome
		emoji  string
	}{
		{model.ReviewApproval, "white_check_mark"},
		{model.ReviewComment, "speech_balloon"},
		{"CHANGES_REQUESTED", ""},
		{"DISMISSED", ""},
		{"", ""},
	} {
		t.Run(string(tc.review), func(t *testing.T) {
			api := &reactingReader{}
			link := "https://github.com/acme/api/pull/42"
			service := &Service{API: api, Data: model.MattermostData{Channels: []model.MattermostChannel{{ID: "c", Selected: true}}, Posts: []model.MattermostPost{{ID: "p", ChannelID: "c", Message: link}}}}
			if err := service.CompleteReview(context.Background(), "p", link, tc.review); err != nil {
				t.Fatal(err)
			}
			if api.emoji != tc.emoji || tc.emoji == "" && api.post != "" {
				t.Fatal("wrong review reaction", api)
			}
		})
	}
}
