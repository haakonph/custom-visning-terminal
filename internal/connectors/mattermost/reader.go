package mattermost

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/haakonph/custom-visning-terminal/internal/model"
	mm "github.com/mattermost/mattermost/server/public/model"
)

var ErrSession = errors.New("Mattermost session expired; sign in again")
var ErrAccount = errors.New("Mattermost token belongs to a different username; cache access refused")
var ErrAccess = errors.New("Mattermost channel access denied")

// API uses the official SDK for authentication and official wire models throughout.
// Read requests are kept here to support bounded pagination and cancellation uniformly.
type API struct {
	sdk     *mm.Client4
	account string
}

func NewAPI(server string) (*API, error) {
	u, err := url.Parse(strings.TrimRight(server, "/"))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("Mattermost server must be an HTTPS URL")
	}
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	sdk := mm.NewAPIv4Client(u.String())
	sdk.HTTPClient = client
	return &API{sdk: sdk}, nil
}
func (a *API) SetToken(token string) { a.sdk.SetToken(token) }
func (a *API) Login(ctx context.Context, user, password, totp string) (string, error) {
	var err error
	if totp != "" {
		_, _, err = a.sdk.LoginWithMFA(ctx, user, password, totp)
	} else {
		_, _, err = a.sdk.Login(ctx, user, password)
	}
	if err != nil {
		return "", errors.New("Mattermost login failed; check credentials and TOTP")
	}
	return a.sdk.AuthToken, nil
}
func apiError(err error) error {
	if err == nil {
		return nil
	}
	var app *mm.AppError
	if errors.As(err, &app) {
		switch app.StatusCode {
		case 401:
			return ErrSession
		case 403:
			return ErrAccess
		}
		return fmt.Errorf("Mattermost HTTP %d", app.StatusCode)
	}
	return errors.New("Mattermost unavailable; cached messages remain available")
}
func (a *API) Me(ctx context.Context) (string, error) {
	u, _, err := a.sdk.GetMe(ctx, "")
	if err != nil {
		return "", apiError(err)
	}
	if a.account != "" && !strings.EqualFold(u.Username, a.account) {
		return "", ErrAccount
	}
	return u.Id, nil
}
func (a *API) Channels(ctx context.Context) ([]model.MattermostChannel, error) {
	teams, _, err := a.sdk.GetTeamsForUser(ctx, "me", "")
	if err != nil {
		return nil, apiError(err)
	}
	var out []model.MattermostChannel
	for _, t := range teams {
		channels, _, err := a.sdk.GetChannelsForTeamForUser(ctx, t.Id, "me", false, "")
		if err != nil {
			return nil, apiError(err)
		}
		// Include public channels in the team even when the user hasn't joined.
		known := map[string]bool{}
		for _, c := range channels {
			known[c.Id] = true
		}
		for page := 0; ; page++ {
			public, _, err := a.sdk.GetPublicChannelsForTeam(ctx, t.Id, page, 200, "")
			if err != nil {
				return nil, apiError(err)
			}
			for _, c := range public {
				if !known[c.Id] {
					channels = append(channels, c)
					known[c.Id] = true
				}
			}
			if len(public) < 200 {
				break
			}
		}
		members, _, err := a.sdk.GetChannelMembersForUser(ctx, "me", t.Id, "")
		if err != nil {
			return nil, apiError(err)
		}
		read := map[string]int64{}
		for _, member := range members {
			read[member.ChannelId] = member.LastViewedAt
		}
		for _, c := range channels {
			if c.DeleteAt == 0 && (c.Type == "O" || c.Type == "P") {
				out = append(out, model.MattermostChannel{ID: c.Id, TeamID: t.Id, Name: c.Name, DisplayName: t.DisplayName + " / " + c.DisplayName, ReadAt: read[c.Id]})
			}
		}
	}
	return out, nil
}
func (a *API) Posts(ctx context.Context, channel string, since int64) ([]model.MattermostPost, error) {
	var list *mm.PostList
	var err error
	if since == 0 {
		list, _, err = a.sdk.GetPostsForChannel(ctx, channel, 0, 60, "", false, false)
	} else {
		list, _, err = a.sdk.GetPostsSince(ctx, channel, max(int64(0), since-1), false)
	}
	if err != nil {
		return nil, apiError(err)
	}
	return mapList(list), nil
}

func (a *API) Thread(ctx context.Context, id string) ([]model.MattermostPost, error) {
	list, _, err := a.sdk.GetPostThread(ctx, id, "", false)
	if err != nil {
		return nil, apiError(err)
	}
	return mapList(list), nil
}
func (a *API) History(ctx context.Context, channel, before string) ([]model.MattermostPost, error) {
	list, _, err := a.sdk.GetPostsBefore(ctx, channel, before, 0, 60, "", false, false)
	if err != nil {
		return nil, apiError(err)
	}
	return mapList(list), nil
}
func (a *API) Author(ctx context.Context, id string) (string, error) {
	u, _, err := a.sdk.GetUser(ctx, id, "")
	if err != nil {
		return "", apiError(err)
	}
	return u.Username, nil
}
func mapList(list *mm.PostList) []model.MattermostPost {
	var posts []*mm.Post
	for _, p := range list.Posts {
		posts = append(posts, p)
	}
	return mapPosts(posts)
}
func mapPosts(posts []*mm.Post) []model.MattermostPost {
	out := make([]model.MattermostPost, 0, len(posts))
	for _, p := range posts {
		out = append(out, model.MattermostPost{ID: p.Id, ChannelID: p.ChannelId, RootID: p.RootId, UserID: p.UserId, Message: p.Message, CreatedAt: p.CreateAt, UpdatedAt: p.UpdateAt, DeletedAt: p.DeleteAt})
	}
	return out
}

func (a *API) ExpectAccount(account string) { a.account = account }
