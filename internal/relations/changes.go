package relations

import (
	"fmt"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"reflect"
)

type LinkChange struct{ Before, After *model.EntityLink }
type ArchiveChange struct{ Before, After *model.TaskReference }
type Changes struct {
	Links   []LinkChange
	Archive []ArchiveChange
}

func ReferenceID(r model.TaskReference) string { return r.Provider + ":" + r.URL }
func LinkID(e model.EntityLink) string         { return ReferenceID(e.Left) + "\n" + ReferenceID(e.Right) }
func Diff(before, after model.EntityState) (Changes, error) {
	result := Changes{}
	oldLinks := map[string]model.EntityLink{}
	newLinks := map[string]model.EntityLink{}
	for _, e := range before.Links {
		oldLinks[LinkID(e)] = e
	}
	for _, e := range after.Links {
		id := LinkID(e)
		if _, ok := newLinks[id]; ok {
			return result, fmt.Errorf("duplisert kobling")
		}
		newLinks[id] = e
		if old, ok := oldLinks[id]; !ok {
			v := e
			result.Links = append(result.Links, LinkChange{After: &v})
		} else if !reflect.DeepEqual(old, e) {
			v, w := old, e
			result.Links = append(result.Links, LinkChange{Before: &v, After: &w})
		}
	}
	for id, e := range oldLinks {
		if _, ok := newLinks[id]; !ok {
			v := e
			result.Links = append(result.Links, LinkChange{Before: &v})
		}
	}
	oldArchive := map[string]model.TaskReference{}
	newArchive := map[string]bool{}
	for _, r := range before.Archived {
		oldArchive[ReferenceID(r)] = r
	}
	for _, r := range after.Archived {
		id := ReferenceID(r)
		newArchive[id] = true
		if old, ok := oldArchive[id]; !ok {
			v := r
			result.Archive = append(result.Archive, ArchiveChange{After: &v})
		} else if !reflect.DeepEqual(old, r) {
			v, w := old, r
			result.Archive = append(result.Archive, ArchiveChange{Before: &v, After: &w})
		}
	}
	for id, r := range oldArchive {
		if !newArchive[id] {
			v := r
			result.Archive = append(result.Archive, ArchiveChange{Before: &v})
		}
	}
	return result, nil
}
