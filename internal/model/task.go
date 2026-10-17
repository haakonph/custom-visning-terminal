package model

// Task is a user-defined workspace, independent of ownership and provider status.
// References are explicit: a task can wrap one item or collect several providers.
type Task struct {
	Title      string          `json:"title"`
	References []TaskReference `json:"references"`
}

type TaskReference struct {
	Provider string `json:"provider"`
	URL      string `json:"url"`
	Key      string `json:"key"`
	Title    string `json:"title"`
	Role     string `json:"role,omitempty"`
}

// Add keeps a single reference per provider URL, updating its display snapshot.
func (t *Task) Add(ref TaskReference) {
	for i, existing := range t.References {
		if existing.Provider == ref.Provider && existing.URL == ref.URL {
			if ref.Role == "" {
				ref.Role = existing.Role
			}
			t.References[i] = ref
			return
		}
	}
	t.References = append(t.References, ref)
}
