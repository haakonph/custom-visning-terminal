package storage

import (
	"context"

	"github.com/haakonph/custom-visning-terminal/internal/storage/dbgen"
)

// Folder holds durable local checkout metadata and display preferences.
type Folder struct {
	Path, Repository, IssueKey, Title string
	Favorite, Hidden                  bool
}

func LoadFolders(path string) ([]Folder, error) {
	db, err := open(path)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := dbgen.New(db).ListLocalFolders(context.Background())
	if err != nil {
		return nil, err
	}
	result := make([]Folder, 0, len(rows))
	for _, f := range rows {
		result = append(result, Folder{Path: f.Path, Repository: f.Repository, IssueKey: f.IssueKey, Title: f.Title, Favorite: f.Favorite != 0, Hidden: f.Hidden != 0})
	}
	return result, nil
}

func SaveFolder(path string, f Folder) error {
	db, err := open(path)
	if err != nil {
		return err
	}
	defer db.Close()
	var favorite, hidden int64
	if f.Favorite {
		favorite = 1
	}
	if f.Hidden {
		hidden = 1
	}
	return dbgen.New(db).SaveLocalFolder(context.Background(), dbgen.SaveLocalFolderParams{Path: f.Path, Repository: f.Repository, IssueKey: f.IssueKey, Title: f.Title, Favorite: favorite, Hidden: hidden})
}
