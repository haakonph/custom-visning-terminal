package model

type LocalStatus struct {
	Path     string `json:"path"`
	Head     string `json:"head"`
	Modified bool   `json:"-"`
	Outdated bool   `json:"-"`
	Error    string `json:"-"`
	Note     string `json:"-"`
}

func (s LocalStatus) String() string {
	if s.Error != "" {
		return "Error: " + s.Error
	}
	if s.Path == "" {
		return "Not downloaded"
	}
	v := "Ready"
	if s.Modified {
		v = "Modified"
	}
	if s.Outdated {
		v += " · Outdated"
	}
	if s.Note != "" {
		v += " · " + s.Note
	}
	return v
}
