package model

import "time"

type SnoozeEntry struct {
	ID    string    `json:"id"`
	Kind  string    `json:"kind"`
	Title string    `json:"title"`
	URL   string    `json:"url,omitempty"`
	Until time.Time `json:"until"`
}
