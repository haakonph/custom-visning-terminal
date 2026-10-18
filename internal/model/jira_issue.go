package model

import "time"

type JiraIssue struct {
	Key, URL, Title, Status, Priority, Assignee, Project, Type, Description, Due string
	Labels                                                                       []string
	Updated                                                                      time.Time
}

type JiraCache struct {
	Query    string
	Issues   []JiraIssue
	SyncedAt time.Time
}
