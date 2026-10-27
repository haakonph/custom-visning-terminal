package snooze

import (
	"github.com/haakonph/custom-visning-terminal/internal/localstate"
	"reflect"
)

type Change = localstate.SnoozeChange

func Changes(before, after map[string]Entry) []Change {
	var changes []Change
	for id, e := range after {
		old, ok := before[id]
		if ok && reflect.DeepEqual(old, e) {
			continue
		}
		next := cloneEntry(e)
		c := Change{After: &next}
		if ok {
			v := cloneEntry(old)
			c.Before = &v
		}
		changes = append(changes, c)
	}
	for id, e := range before {
		if _, ok := after[id]; !ok {
			v := cloneEntry(e)
			changes = append(changes, Change{Before: &v})
		}
	}
	return changes
}
func cloneEntry(e Entry) Entry {
	e.Roots = append(e.Roots[:0:0], e.Roots...)
	e.Members = append(e.Members[:0:0], e.Members...)
	return e
}
