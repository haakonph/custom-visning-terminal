package relations

import "github.com/haakonph/custom-visning-terminal/internal/model"

func Link(state model.EntityState, origins, targets []model.TaskReference) model.EntityState {
	state.Links = append([]model.EntityLink(nil), state.Links...)
	seen := map[string]bool{}
	for _, e := range state.Links {
		seen[LinkID(e)] = true
		seen[ReferenceID(e.Right)+"\n"+ReferenceID(e.Left)] = true
	}
	for _, a := range origins {
		for _, b := range targets {
			left, right := ReferenceID(a), ReferenceID(b)
			if left == right || seen[left+"\n"+right] {
				continue
			}
			state.Links = append(state.Links, model.EntityLink{Left: a, Right: b})
			seen[left+"\n"+right], seen[right+"\n"+left] = true, true
		}
	}
	return state
}
func Archive(state model.EntityState, refs []model.TaskReference, archived bool) model.EntityState {
	state.Archived = append([]model.TaskReference(nil), state.Archived...)
	for _, r := range refs {
		if r.Provider == "task" {
			continue
		}
		var kept []model.TaskReference
		for _, existing := range state.Archived {
			if ReferenceID(existing) != ReferenceID(r) {
				kept = append(kept, existing)
			}
		}
		if archived {
			kept = append(kept, r)
		}
		state.Archived = kept
	}
	return state
}

// Unlink removes incident edges when deleting a task, or just an owner's
// selected edges when removing references from that task.
func Unlink(state model.EntityState, refs []model.TaskReference, ownerID string) model.EntityState {
	remove := map[string]bool{}
	for _, r := range refs {
		remove[ReferenceID(r)] = true
	}
	owner := "task:task://" + ownerID
	var kept []model.EntityLink
	for _, edge := range state.Links {
		left, right := ReferenceID(edge.Left), ReferenceID(edge.Right)
		discard := remove[left] || remove[right]
		if ownerID != "" {
			discard = (left == owner && remove[right]) || (right == owner && remove[left])
		}
		if !discard {
			kept = append(kept, edge)
		}
	}
	state.Links = kept
	return state
}
