package tasks

import "github.com/haakonph/custom-visning-terminal/internal/model"

func Reference(t model.Task) model.TaskReference {
	return model.TaskReference{Provider: "task", URL: "task://" + t.ID, Key: t.ID, Title: t.Title}
}
func HasTasks(refs []model.TaskReference) bool {
	for _, r := range refs {
		if r.Provider == "task" {
			return true
		}
	}
	return false
}
func LinkReferences(items []model.Task, origins, targets []model.TaskReference) []model.Task {
	out := Clone(items)
	for _, a := range origins {
		for _, b := range targets {
			if a.Provider == b.Provider && a.URL == b.URL {
				continue
			}
			for i := range out {
				if a.Provider == "task" && a.Key == out[i].ID {
					out[i].Add(b)
				}
				if b.Provider == "task" && b.Key == out[i].ID {
					out[i].Add(a)
				}
			}
		}
	}
	return out
}
func Archive(items []model.Task, refs []model.TaskReference, archived bool) []model.Task {
	out := Clone(items)
	for _, r := range refs {
		if r.Provider == "task" {
			for i := range out {
				if out[i].ID == r.Key {
					out[i].Archived = archived
				}
			}
		}
	}
	return out
}
func RemoveReferences(items []model.Task, refs []model.TaskReference, ownerID string) []model.Task {
	remove := map[string]bool{}
	for _, r := range refs {
		remove[r.Provider+":"+r.URL] = true
	}
	var out []model.Task
	for _, task := range Clone(items) {
		if ownerID == "" {
			r := Reference(task)
			if remove[r.Provider+":"+r.URL] {
				continue
			}
		}
		if task.ID == ownerID {
			var kept []model.TaskReference
			for _, r := range task.References {
				if !remove[r.Provider+":"+r.URL] {
					kept = append(kept, r)
				}
			}
			task.References = kept
		}
		out = append(out, task)
	}
	return out
}
