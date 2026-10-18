package storage

import (
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

// LoadMattermost reads an account-scoped offline snapshot from the shared SQLite schema.
func LoadMattermost(path, server, user string) (model.MattermostData, error) {
	data := model.MattermostData{Server: server, UserID: user}
	db, err := open(path)
	if err != nil {
		return data, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return data, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id,team_id,name,display_name,selected,synced_at,read_at,last_sync_at FROM mattermost_channels WHERE server=? AND user_id=? ORDER BY display_name,id`, server, user)
	if err != nil {
		return data, err
	}
	for rows.Next() {
		var c model.MattermostChannel
		if err = rows.Scan(&c.ID, &c.TeamID, &c.Name, &c.DisplayName, &c.Selected, &c.SyncedAt, &c.ReadAt, &c.LastSyncAt); err != nil {
			rows.Close()
			return data, err
		}
		data.Channels = append(data.Channels, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return data, err
	}
	rows, err = tx.Query(`SELECT id,channel_id,root_id,sender_id,author,message,created_at,updated_at,deleted_at FROM mattermost_posts WHERE server=? AND user_id=? ORDER BY created_at,id`, server, user)
	if err != nil {
		return data, err
	}
	defer rows.Close()
	for rows.Next() {
		var p model.MattermostPost
		if err = rows.Scan(&p.ID, &p.ChannelID, &p.RootID, &p.UserID, &p.Author, &p.Message, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt); err != nil {
			return data, err
		}
		data.Posts = append(data.Posts, p)
	}
	return data, rows.Err()
}

// SaveMattermost atomically updates the offline snapshot and its reverse PR index.
func SaveMattermost(path string, data model.MattermostData, links map[string][]string) error {
	db, err := open(path)
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT OR IGNORE INTO mattermost_accounts VALUES (?,?)`, data.Server, data.UserID); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM mattermost_channels WHERE server=? AND user_id=?`, data.Server, data.UserID); err != nil {
		return err
	}
	for _, c := range data.Channels {
		if _, err = tx.Exec(`INSERT INTO mattermost_channels VALUES (?,?,?,?,?,?,?,?,?,?)`, data.Server, data.UserID, c.ID, c.TeamID, c.Name, c.DisplayName, c.Selected, c.SyncedAt, c.ReadAt, c.LastSyncAt); err != nil {
			return err
		}
	}
	for _, p := range data.Posts {
		if _, err = tx.Exec(`INSERT INTO mattermost_posts VALUES (?,?,?,?,?,?,?,?,?,?,?)`, data.Server, data.UserID, p.ID, p.ChannelID, p.RootID, p.UserID, p.Author, p.Message, p.CreatedAt, p.UpdatedAt, p.DeletedAt); err != nil {
			return err
		}
		for _, u := range links[p.ID] {
			if _, err = tx.Exec(`INSERT INTO mattermost_pr_mentions VALUES (?,?,?,?)`, data.Server, data.UserID, p.ID, u); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// ClearAllMattermost removes every locally cached Mattermost account and relation.
func ClearAllMattermost(path string) error {
	db, err := open(path)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(`DELETE FROM mattermost_accounts`)
	return err
}
