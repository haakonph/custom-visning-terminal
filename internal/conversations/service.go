package conversations

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/storage"
)

// ReaderAPI is intentionally small so synchronization can be tested without a server.
type ReaderAPI interface {
	Me(context.Context) (string, error)
	Channels(context.Context) ([]model.MattermostChannel, error)
	Posts(context.Context, string, int64) ([]model.MattermostPost, error)
	Thread(context.Context, string) ([]model.MattermostPost, error)
	History(context.Context, string, string) ([]model.MattermostPost, error)
	Author(context.Context, string) (string, error)
}

type Service struct {
	mu   sync.Mutex
	API  ReaderAPI
	Path string
	Data model.MattermostData
}

func OpenService(api ReaderAPI, path, server, account string) (*Service, error) {
	data, err := storage.LoadMattermost(path, server, account)
	return &Service{API: api, Path: path, Data: data}, err
}
func (s *Service) Snapshot() model.MattermostData {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot()
}
func (s *Service) snapshot() model.MattermostData {
	d := s.Data
	d.Channels = append([]model.MattermostChannel(nil), d.Channels...)
	d.Posts = append([]model.MattermostPost(nil), d.Posts...)
	return d
}
func (s *Service) save() error {
	links := map[string][]string{}
	for _, p := range s.Data.Posts {
		if p.DeletedAt == 0 {
			for _, link := range PRLinks(p.Message) {
				links[p.ID] = append(links[p.ID], link.URL)
			}
		}
	}
	return storage.SaveMattermost(s.Path, s.Data, links)
}
func (s *Service) Discover(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.API.Me(ctx); err != nil {
		return err
	}
	channels, err := s.API.Channels(ctx)
	if err != nil {
		return err
	}
	old := map[string]model.MattermostChannel{}
	for _, c := range s.Data.Channels {
		old[c.ID] = c
	}
	for _, c := range channels {
		if prev, ok := old[c.ID]; ok {
			c.Selected = prev.Selected
			c.ReadAt = max(c.ReadAt, prev.ReadAt)
			c.SyncedAt = prev.SyncedAt
			c.LastSyncAt = prev.LastSyncAt
		}
		old[c.ID] = c
	}
	s.Data.Channels = nil
	for _, c := range old {
		s.Data.Channels = append(s.Data.Channels, c)
	}
	sort.Slice(s.Data.Channels, func(i, j int) bool {
		a, b := s.Data.Channels[i], s.Data.Channels[j]
		if a.DisplayName == b.DisplayName {
			return a.ID < b.ID
		}
		return a.DisplayName < b.DisplayName
	})
	return s.save()
}
func (s *Service) Select(id string, selected bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.Data.Channels {
		if s.Data.Channels[i].ID == id {
			s.Data.Channels[i].Selected = selected
			return s.save()
		}
	}
	return errors.New("unknown Mattermost channel")
}
func (s *Service) MarkRead(id string, at int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.Data.Channels {
		if s.Data.Channels[i].ID == id {
			s.Data.Channels[i].ReadAt = max(at, s.Data.Channels[i].ReadAt)
			return s.save()
		}
	}
	return nil
}
func (s *Service) Sync(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.API.Me(ctx); err != nil {
		return err
	}
	var failures []error
	for i := range s.Data.Channels {
		c := &s.Data.Channels[i]
		if !c.Selected {
			continue
		}
		posts, err := s.API.Posts(ctx, c.ID, c.SyncedAt)
		if err != nil {
			failures = append(failures, fmt.Errorf("#%s: %w", c.Name, err))
			if errors.Is(err, ErrSession) {
				break
			}
			continue
		}
		s.merge(ctx, posts)
		c.LastSyncAt = time.Now().UnixMilli()
		for _, p := range posts {
			c.SyncedAt = max(c.SyncedAt, p.CreatedAt, p.UpdatedAt, p.DeletedAt)
		}
	}
	failures = append(failures, s.save())
	return errors.Join(failures...)
}
func (s *Service) More(ctx context.Context, channel, before, root string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.API.Me(ctx); err != nil {
		return err
	}
	var posts []model.MattermostPost
	var err error
	if root != "" {
		posts, err = s.API.Thread(ctx, root)
	} else {
		posts, err = s.API.History(ctx, channel, before)
	}
	if err != nil {
		return err
	}
	s.merge(ctx, posts)
	return s.save()
}
func (s *Service) merge(ctx context.Context, posts []model.MattermostPost) {
	existing := map[string]model.MattermostPost{}
	authors := map[string]string{}
	for _, p := range s.Data.Posts {
		existing[p.ID] = p
		if p.Author != "" {
			authors[p.UserID] = p.Author
		}
	}
	for _, p := range posts {
		prev, ok := existing[p.ID]
		if ok && max(prev.UpdatedAt, prev.DeletedAt) > max(p.UpdatedAt, p.DeletedAt) {
			continue
		}
		if p.DeletedAt != 0 {
			p.Message = ""
		} else {
			p.Author = authors[p.UserID]
			if p.Author == "" {
				p.Author, _ = s.API.Author(ctx, p.UserID)
				if p.Author == "" {
					p.Author = p.UserID
				}
				authors[p.UserID] = p.Author
			}
		}
		existing[p.ID] = p
	}
	s.Data.Posts = nil
	for _, p := range existing {
		s.Data.Posts = append(s.Data.Posts, p)
	}
	sort.Slice(s.Data.Posts, func(i, j int) bool {
		a, b := s.Data.Posts[i], s.Data.Posts[j]
		if a.CreatedAt == b.CreatedAt {
			return a.ID < b.ID
		}
		return a.CreatedAt < b.CreatedAt
	})
}
func (s *Service) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := storage.ClearAllMattermost(s.Path); err != nil {
		return err
	}
	s.Data.Channels = nil
	s.Data.Posts = nil
	return nil
}
