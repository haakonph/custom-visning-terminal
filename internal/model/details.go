package model

import "context"

// DetailSource is optional so cached/list-only providers remain usable.
type DetailSource interface {
	Details(context.Context, PullRequest) (PRDetails, error)
}
type ChangedFile struct {
	Path, PreviousPath, Status string
	Additions, Deletions       int
}
type PRComment struct {
	Author, Body, Path string
	Line               int
}
type ActionCheck struct{ Name, Status, Conclusion string }
type PRDetails struct {
	// PullRequest is present only when current status was fetched successfully.
	PullRequest *PullRequest
	Files       []ChangedFile
	Comments    []PRComment
	Checks      []ActionCheck
	Warnings    []string
}
