package storage

import "time"

// SaveReviewMessages records successful sends atomically without retaining message content.
func SaveReviewMessages(path string, links []string, at time.Time) error {
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
	for _, link := range links {
		if _, err := tx.Exec(`INSERT INTO review_messages(pull_request_url,sent_at) VALUES(?,?) ON CONFLICT(pull_request_url) DO UPDATE SET sent_at=excluded.sent_at WHERE excluded.sent_at > sent_at`, link, at.UTC().Format("2006-01-02T15:04:05.000000000Z")); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func LoadReviewMessages(path string) (map[string]time.Time, error) {
	db, err := open(path)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT pull_request_url,sent_at FROM review_messages`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sent := map[string]time.Time{}
	for rows.Next() {
		var link, timestamp string
		if err := rows.Scan(&link, &timestamp); err != nil {
			return nil, err
		}
		at, err := time.Parse(time.RFC3339Nano, timestamp)
		if err != nil {
			return nil, err
		}
		sent[link] = at
	}
	return sent, rows.Err()
}
