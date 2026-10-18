CREATE TABLE mattermost_accounts (
 server TEXT NOT NULL, user_id TEXT NOT NULL,
 PRIMARY KEY(server,user_id)
);
CREATE TABLE mattermost_channels (
 server TEXT NOT NULL, user_id TEXT NOT NULL, id TEXT NOT NULL,
 team_id TEXT NOT NULL, name TEXT NOT NULL, display_name TEXT NOT NULL,
 selected INTEGER NOT NULL, synced_at INTEGER NOT NULL, read_at INTEGER NOT NULL, last_sync_at INTEGER NOT NULL,
 PRIMARY KEY(server,user_id,id),
 FOREIGN KEY(server,user_id) REFERENCES mattermost_accounts ON DELETE CASCADE
);
CREATE TABLE mattermost_posts (
 server TEXT NOT NULL, user_id TEXT NOT NULL, id TEXT NOT NULL, channel_id TEXT NOT NULL,
 root_id TEXT NOT NULL, sender_id TEXT NOT NULL, author TEXT NOT NULL, message TEXT NOT NULL,
 created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, deleted_at INTEGER NOT NULL,
 PRIMARY KEY(server,user_id,id),
 FOREIGN KEY(server,user_id,channel_id) REFERENCES mattermost_channels(server,user_id,id) ON DELETE CASCADE
);
CREATE TABLE mattermost_pr_mentions (
 server TEXT NOT NULL, user_id TEXT NOT NULL, post_id TEXT NOT NULL, pr_url TEXT NOT NULL,
 PRIMARY KEY(server,user_id,post_id,pr_url),
 FOREIGN KEY(server,user_id,post_id) REFERENCES mattermost_posts(server,user_id,id) ON DELETE CASCADE
);
CREATE INDEX mattermost_mentions_url ON mattermost_pr_mentions(pr_url);
CREATE INDEX mattermost_posts_channel ON mattermost_posts(server,user_id,channel_id,created_at);
