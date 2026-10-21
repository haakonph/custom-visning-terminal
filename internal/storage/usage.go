package storage

import "database/sql"

// UsageKey contains only dates and application-defined labels, never user content.
type UsageKey struct {
	Day, Category, Name, Source, Target string
}

type UsageCount struct {
	UsageKey
	Count int64
}

// UsageStore keeps a connection open so recording does not rerun migrations.
type UsageStore struct{ db *sql.DB }

func OpenUsage(path string) (*UsageStore, error) {
	db, err := open(path)
	if err != nil {
		return nil, err
	}
	return &UsageStore{db: db}, nil
}

func (s *UsageStore) Close() error { return s.db.Close() }

// Add increments counters atomically, including when multiple app instances run.
func (s *UsageStore) Add(counts map[UsageKey]int64) error {
	if len(counts) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO usage_metrics(day,category,name,source,target,count) VALUES(?,?,?,?,?,?)
		ON CONFLICT(day,category,name,source,target) DO UPDATE SET count=count+excluded.count`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for k, n := range counts {
		if _, err = stmt.Exec(k.Day, k.Category, k.Name, k.Source, k.Target, n); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *UsageStore) Counts() ([]UsageCount, error) {
	rows, err := s.db.Query(`SELECT day,category,name,source,target,count FROM usage_metrics ORDER BY day,category,name,source,target`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var counts []UsageCount
	for rows.Next() {
		var c UsageCount
		if err := rows.Scan(&c.Day, &c.Category, &c.Name, &c.Source, &c.Target, &c.Count); err != nil {
			return nil, err
		}
		counts = append(counts, c)
	}
	return counts, rows.Err()
}
