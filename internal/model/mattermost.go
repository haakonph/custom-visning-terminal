package model

// Mattermost data contains no credentials. IDs are scoped by the cache's server/account.
type MattermostChannel struct {
	ID, TeamID, Name, DisplayName string
	Selected                      bool
	SyncedAt, ReadAt, LastSyncAt  int64
}
type MattermostPost struct {
	ID, ChannelID, RootID, UserID, Author, Message string
	CreatedAt, UpdatedAt, DeletedAt                int64
}
type MattermostData struct {
	Server, UserID string
	Channels       []MattermostChannel
	Posts          []MattermostPost
}
type PRLink struct {
	Owner, Repository string
	Number            int
	URL               string
}
