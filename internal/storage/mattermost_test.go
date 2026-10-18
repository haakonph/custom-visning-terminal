package storage

import (
	"path/filepath"
	"testing"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestMattermostRelationsAndIsolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.sqlite")
	data := model.MattermostData{Server: "https://chat.example", UserID: "alice", Channels: []model.MattermostChannel{{ID: "c", Name: "private", Selected: true}, {ID: "d", Name: "public", Selected: true}}, Posts: []model.MattermostPost{{ID: "p", ChannelID: "c", Message: "first"}, {ID: "q", ChannelID: "d", RootID: "p", Message: "reply"}}}
	links := map[string][]string{"p": {"https://github.com/a/b/pull/1", "https://github.com/a/b/pull/2"}, "q": {"https://github.com/a/b/pull/1"}}
	if err := SaveMattermost(path, data, links); err != nil {
		t.Fatal(err)
	}
	db, err := open(path)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM mattermost_pr_mentions WHERE pr_url=?`, links["p"][0]).Scan(&count); err != nil || count != 2 {
		t.Fatal(count, err)
	}
	db.Close()
	other, err := LoadMattermost(path, data.Server, "bob")
	if err != nil || len(other.Posts) != 0 {
		t.Fatal(other, err)
	}
	other, err = LoadMattermost(path, "https://other.example", "alice")
	if err != nil || len(other.Posts) != 0 {
		t.Fatal(other, err)
	}
	delete(links, "p")
	data.Posts[0].DeletedAt = 10
	data.Posts[0].Message = ""
	if err = SaveMattermost(path, data, links); err != nil {
		t.Fatal(err)
	}
	db, err = open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.QueryRow(`SELECT count(*) FROM mattermost_pr_mentions`).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
}
