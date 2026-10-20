package mattermost

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestPRLinks(t *testing.T) {
	tests := []struct {
		text string
		want []string
	}{
		{"https://github.com/Org/Repo/pull/12 https://github.com/org/repo/pull/12/files https://github.com/org/repo/pull/12", []string{"https://github.com/org/repo/pull/12"}},
		{"[one](https://github.com/a/b/pull/1) <https://github.com/a/c/pull/2#discussion> https://github.com/a/b/pull/1?x=2", []string{"https://github.com/a/b/pull/1", "https://github.com/a/c/pull/2"}},
		{"http://github.com/a/b/pull/1 https://github.com.evil/a/b/pull/1 https://github.com/a/b/pull/0 https://github.com/a/b/issues/1 https://github.com/a/b/pull/12bad", nil},
		{"https://github.com/a/b/pull/9999999999999999999999999999999999", nil},
	}
	for _, tt := range tests {
		var got []string
		for _, l := range PRLinks(tt.text) {
			got = append(got, l.URL)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%q: %v, want %v", tt.text, got, tt.want)
		}
	}
}

type fakeReader struct {
	channels   []model.MattermostChannel
	posts      []model.MattermostPost
	since      []int64
	err        error
	postErrors map[string]error
}

func (f *fakeReader) Me(context.Context) (string, error) { return "user", f.err }
func (f *fakeReader) Channels(context.Context) ([]model.MattermostChannel, error) {
	return f.channels, f.err
}
func (f *fakeReader) Posts(_ context.Context, channel string, since int64) ([]model.MattermostPost, error) {
	f.since = append(f.since, since)
	if err := f.postErrors[channel]; err != nil {
		return nil, err
	}
	return f.posts, f.err
}
func (f *fakeReader) Thread(context.Context, string) ([]model.MattermostPost, error) {
	return f.posts, f.err
}
func (f *fakeReader) History(context.Context, string, string) ([]model.MattermostPost, error) {
	return f.posts, f.err
}
func (f *fakeReader) Author(context.Context, string) (string, error) { return "Ola", nil }

func TestSubscriptionsIncrementalEditsDeletesOffline(t *testing.T) {
	ctx := context.Background()
	f := &fakeReader{channels: []model.MattermostChannel{{ID: "c", Name: "backend"}, {ID: "other", Name: "private"}}}
	path := filepath.Join(t.TempDir(), "mattermost.sqlite")
	s, err := OpenService(f, path, "https://chat.example", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Discover(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.Select("c", true); err != nil {
		t.Fatal(err)
	}
	link := "https://github.com/org/repo/pull/1"
	f.posts = []model.MattermostPost{{ID: "p", ChannelID: "c", UserID: "u", Message: link, CreatedAt: 100, UpdatedAt: 100}}
	if err = s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	f.posts = []model.MattermostPost{{ID: "p", ChannelID: "c", UserID: "u", Message: "edited " + link, CreatedAt: 100, UpdatedAt: 200}, {ID: "reply", RootID: "p", ChannelID: "c", UserID: "u", Message: link, CreatedAt: 201, UpdatedAt: 201}}
	if err = s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.since, []int64{0, 100}) {
		t.Fatal(f.since)
	}
	if len(s.Data.Posts) != 2 || s.Data.Posts[0].Message != "edited "+link {
		t.Fatal(s.Data.Posts)
	}
	if err = s.MarkRead("c", 201); err != nil {
		t.Fatal(err)
	}
	f.posts = []model.MattermostPost{{ID: "p", ChannelID: "c", CreatedAt: 100, UpdatedAt: 300, DeletedAt: 300, Message: "deleted secret"}}
	if err = s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if s.Data.Posts[0].Message != "" || s.Data.Posts[0].DeletedAt != 300 {
		t.Fatal(s.Data.Posts)
	}
	reloaded, err := OpenService(f, path, s.Data.Server, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reloaded.Data, s.Data) {
		t.Fatalf("offline snapshot differs: %+v / %+v", reloaded.Data, s.Data)
	}
	f.err = ErrSession
	before := reloaded.Snapshot()
	if err = reloaded.Sync(ctx); !errors.Is(err, ErrSession) {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, reloaded.Snapshot()) {
		t.Fatal("failed sync damaged cache")
	}
	f.err = nil
	if err = reloaded.Select("c", false); err != nil {
		t.Fatal(err)
	}
	if err = reloaded.Clear(); err != nil {
		t.Fatal(err)
	}
	empty, err := OpenService(f, path, s.Data.Server, "alice")
	if err != nil || len(empty.Data.Channels) != 0 || len(empty.Data.Posts) != 0 {
		t.Fatal(empty, err)
	}
}
func TestAPIAuthenticationAndErrors(t *testing.T) {
	for _, status := range []int{200, 401, 403, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.EqualFold(r.Header.Get("Authorization"), "Bearer test-token") {
					t.Error("missing token")
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				if status == 200 {
					_, _ = w.Write([]byte(`{"id":"user"}`))
				} else {
					_ = json.NewEncoder(w).Encode(map[string]any{"id": "test.error", "message": "sensitive upstream body", "status_code": status})
				}
			}))
			defer srv.Close()
			api, err := NewAPI(srv.URL)
			if err != nil {
				t.Fatal(err)
			}
			api.sdk.HTTPClient = srv.Client()
			api.SetToken("test-token")
			id, err := api.Me(context.Background())
			switch status {
			case 200:
				if err != nil || id != "user" {
					t.Fatal(id, err)
				}
			case 401:
				if !errors.Is(err, ErrSession) {
					t.Fatal(err)
				}
			case 403:
				if !errors.Is(err, ErrAccess) {
					t.Fatal(err)
				}
			default:
				if err == nil || strings.Contains(err.Error(), "sensitive") {
					t.Fatal(err)
				}
			}
		})
	}
}
func TestLoginTOTP(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/users/login" {
			t.Fatal(r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["login_id"] != "alice" || body["password"] != "password" || body["token"] != "123456" {
			t.Errorf("unexpected login fields: %v", body)
		}
		w.Header().Set("Token", "session-token")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"u"}`))
	}))
	defer srv.Close()
	a, err := NewAPI(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	a.sdk.HTTPClient = srv.Client()
	token, err := a.Login(context.Background(), "alice", "password", "123456")
	if err != nil || token != "session-token" {
		t.Fatal(token, err)
	}
}
func TestServerValidation(t *testing.T) {
	for _, server := range []string{"http://chat.example", "https://user:password@chat.example", "https://chat.example?token=secret", "https://chat.example#fragment", ""} {
		if _, err := NewAPI(server); err == nil {
			t.Error(server)
		}
	}
}

func TestAPIRejectsDifferentAccount(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"bob-id","username":"bob"}`))
	}))
	defer srv.Close()
	a, _ := NewAPI(srv.URL)
	a.sdk.HTTPClient = srv.Client()
	a.ExpectAccount("alice")
	a.SetToken("bob-token")
	if _, err := a.Me(context.Background()); !errors.Is(err, ErrAccount) {
		t.Fatal(err)
	}
}

func TestDiscoveryRequiresValidSession(t *testing.T) {
	f := &fakeReader{err: ErrSession}
	s, err := OpenService(f, filepath.Join(t.TempDir(), "cache.sqlite"), "https://chat.example", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Discover(context.Background()); !errors.Is(err, ErrSession) {
		t.Fatal(err)
	}
	if err = s.More(context.Background(), "c", "p", ""); !errors.Is(err, ErrSession) {
		t.Fatal(err)
	}
	if err = s.Select("inaccessible", true); err == nil {
		t.Fatal("unknown channel accepted")
	}
}

func TestPartialAccessFailureKeepsSuccessfulChannels(t *testing.T) {
	f := &fakeReader{channels: []model.MattermostChannel{{ID: "c", Name: "a"}, {ID: "d", Name: "b"}}, postErrors: map[string]error{"d": ErrAccess}, posts: []model.MattermostPost{{ID: "p", ChannelID: "c", UserID: "u", CreatedAt: 100, UpdatedAt: 100}}}
	s, err := OpenService(f, filepath.Join(t.TempDir(), "cache.sqlite"), "https://chat.example", "alice")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err = s.Discover(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.Select("c", true); err != nil {
		t.Fatal(err)
	}
	if err = s.Select("d", true); err != nil {
		t.Fatal(err)
	}
	if err = s.Sync(ctx); !errors.Is(err, ErrAccess) || !strings.Contains(err.Error(), "#b:") {
		t.Fatal(err)
	}
	if len(s.Data.Posts) != 1 || s.Data.Channels[0].SyncedAt != 100 || s.Data.Channels[1].SyncedAt != 0 {
		t.Fatal(s.Data)
	}
	reloaded, err := OpenService(f, s.Path, s.Data.Server, s.Data.UserID)
	if err != nil || len(reloaded.Data.Posts) != 1 {
		t.Fatal(reloaded, err)
	}
	// An older response cannot undo a more recent edit.
	f.posts[0].Message = "fresh"
	f.posts[0].UpdatedAt = 200
	if err = s.Sync(ctx); !errors.Is(err, ErrAccess) {
		t.Fatal(err)
	}
	f.posts[0].Message = "stale"
	f.posts[0].UpdatedAt = 150
	if err = s.Sync(ctx); !errors.Is(err, ErrAccess) {
		t.Fatal(err)
	}
	if s.Data.Posts[0].Message != "fresh" {
		t.Fatal(s.Data.Posts)
	}
}

func TestPostsNeverRequestDeletedMessages(t *testing.T) {
	for _, operation := range []string{"initial", "since", "history", "thread"} {
		for _, status := range []int{http.StatusOK, http.StatusForbidden, http.StatusUnauthorized, http.StatusInternalServerError} {
			t.Run(operation+"/"+http.StatusText(status), func(t *testing.T) {
				var queries []string
				srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					queries = append(queries, r.URL.RawQuery)
					wantPath := "/api/v4/channels/c/posts"
					if operation == "thread" {
						wantPath = "/api/v4/posts/p/thread"
					}
					if r.URL.Path != wantPath || !strings.EqualFold(r.Header.Get("Authorization"), "Bearer test-token") {
						t.Error("unexpected request", r.URL, r.Header.Get("Authorization"))
					}
					q := r.URL.Query()
					if q.Has("include_deleted") {
						t.Error("request includes deleted messages", r.URL)
					}
					if operation == "since" && q.Get("since") != "99" {
						t.Error("lost incremental cursor")
					}
					if operation == "history" && q.Get("before") != "p" {
						t.Error("lost history cursor")
					}
					w.Header().Set("Content-Type", "application/json")
					code := status
					if code != http.StatusOK {
						w.WriteHeader(code)
						_ = json.NewEncoder(w).Encode(map[string]any{"id": "test.error", "message": "sensitive upstream body", "status_code": code})
						return
					}
					_, _ = w.Write([]byte(`{"order":["p"],"posts":{"p":{"id":"p","channel_id":"c","message":"visible"}}}`))
				}))
				defer srv.Close()
				a, _ := NewAPI(srv.URL)
				a.sdk.HTTPClient = srv.Client()
				a.SetToken("test-token")
				var posts []model.MattermostPost
				var err error
				switch operation {
				case "initial":
					posts, err = a.Posts(context.Background(), "c", 0)
				case "since":
					posts, err = a.Posts(context.Background(), "c", 100)
				case "history":
					posts, err = a.History(context.Background(), "c", "p")
				case "thread":
					posts, err = a.Thread(context.Background(), "p")
				}
				if len(queries) != 1 {
					t.Fatalf("requests = %v, want exactly one", queries)
				}
				switch status {
				case http.StatusOK:
					if err != nil || len(posts) != 1 || posts[0].Message != "visible" {
						t.Fatal(posts, err)
					}
				case http.StatusForbidden:
					if !errors.Is(err, ErrAccess) {
						t.Fatal(err)
					}
				case http.StatusUnauthorized:
					if !errors.Is(err, ErrSession) {
						t.Fatal(err)
					}
				default:
					if err == nil || strings.Contains(err.Error(), "sensitive") {
						t.Fatal(err)
					}
				}
			})
		}
	}
}
