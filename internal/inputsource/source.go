// Package inputsource reads the active operating-system input source.
// A keyboard's firmware layout is not exposed by this API.
package inputsource

type Source struct {
	ID   string
	Name string
}
