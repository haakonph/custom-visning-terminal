// Package model contains provider-independent dashboard data.
package model

import (
	"context"
	"sort"
	"time"
)

type View int

const (
	Open View = iota
	Drafts
	Reviews
	TeamReviews
	ViewCount
)

func (v View) String() string {
	return [...]string{"My Open PRs", "My Draft PRs", "My Review Requests", "Team Review Requests"}[v]
}

type Status string

const (
	Unknown          Status = "UNKNOWN"
	NoReview         Status = "NO REVIEW"
	Approved         Status = "APPROVED"
	Pending          Status = "PENDING"
	ChangesRequested Status = "CHANGES"
	Pass             Status = "PASS"
	Fail             Status = "FAIL"
	Warning          Status = "WARNING"
	Texts            Status = "TEKSTER"
	Mergeable        Status = "MERGEABLE"
	Conflicting      Status = "CONFLICT"
)

type PullRequest struct {
	// OpenComments is the number of unresolved review threads; nil means unavailable.
	OpenComments       *int           `json:"open_comments,omitempty"`
	ID                 string         `json:"id"`
	Repository         string         `json:"repository"`
	Number             int            `json:"number"`
	Body               string         `json:"body,omitempty"`
	HeadOID            string         `json:"head_oid,omitempty"`
	BaseRef            string         `json:"base_ref,omitempty"`
	HeadRef            string         `json:"head_ref,omitempty"`
	Title              string         `json:"title"`
	URL                string         `json:"url"`
	Author             string         `json:"author"`
	Draft              bool           `json:"draft"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	Labels             []string       `json:"labels"`
	RequestedReviewers []string       `json:"requested_reviewers"`
	RequestedTeams     []string       `json:"requested_teams"`
	CopilotReview      *CopilotReview `json:"copilot_review,omitempty"`
	ReviewSummary      *ReviewSummary `json:"review_summary,omitempty"`
	Review             Status         `json:"review"`
	CI                 Status         `json:"ci"`
	AutoMerge          Status         `json:"auto_merge,omitempty"`
	MergeQueue         Status         `json:"merge_queue,omitempty"`
	QueuePosition      int            `json:"queue_position,omitempty"`
	Mergeability       Status         `json:"mergeability"`
}

type CopilotReview struct {
	Recommendation string `json:"recommendation"`
	Comments       int    `json:"comments"`
}

// ReviewSummary counts current opinions per reviewer, with outstanding requests
// counted separately (a reviewer can be asked to review again).
type ReviewSummary struct {
	Approved     int  `json:"approved"`
	Changes      int  `json:"changes"`
	Pending      int  `json:"pending"`
	PendingTeams int  `json:"pending_teams"`
	Complete     bool `json:"complete"`
	Fallback     bool `json:"fallback"`
}

func (p PullRequest) Key() string { return p.URL }

type Result struct {
	PRs      []PullRequest
	Complete bool
	Warnings []string
}

// Source is implemented by connectors; the TUI never imports a provider SDK.
type Source interface {
	User(context.Context) (string, error)
	List(context.Context, View, string) (Result, error)
}

// AutoMergeSource changes GitHub's merge-when-ready setting and returns the
// server's current merge status, including merge queue membership.
type AutoMergeSource interface {
	SetAutoMerge(context.Context, PullRequest, bool) (PullRequest, error)
}
type Dataset struct {
	PRs      []PullRequest `json:"prs"`
	SyncedAt time.Time     `json:"synced_at"`
}
type Snapshot struct {
	Version  int                `json:"version"`
	User     string             `json:"user"`
	Datasets [ViewCount]Dataset `json:"datasets"`
}

func Sort(prs []PullRequest) {
	sort.Slice(prs, func(i, j int) bool {
		if prs[i].UpdatedAt.Equal(prs[j].UpdatedAt) {
			return prs[i].URL < prs[j].URL
		}
		return prs[i].UpdatedAt.After(prs[j].UpdatedAt)
	})
}

// Merge preserves existing rows when an API response is incomplete.
func Merge(old, fresh []PullRequest) []PullRequest {
	rows := make(map[string]PullRequest, len(old)+len(fresh))
	for _, p := range old {
		rows[p.Key()] = p
	}
	for _, p := range fresh {
		rows[p.Key()] = p
	}
	out := make([]PullRequest, 0, len(rows))
	for _, p := range rows {
		out = append(out, p)
	}
	Sort(out)
	return out
}

// CopilotReviewSource requests a new GitHub Copilot code review.
type CopilotReviewSource interface {
	RequestCopilotReview(context.Context, PullRequest) error
}
