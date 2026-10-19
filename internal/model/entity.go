package model

// EntityLink is a local, bidirectional association; it never edits an upstream system.
type EntityLink struct{ Left, Right TaskReference }
type EntityState struct {
	Links    []EntityLink
	Archived []TaskReference
}
