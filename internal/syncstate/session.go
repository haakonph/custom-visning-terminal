// Package syncstate supplies request identity and scheduling, without provider rules.
// Owners call these methods on their serialized event loop; workers only hold Tokens.
package syncstate

import (
	"context"
	"time"
)

type Token struct {
	Generation uint64
	Sequence   uint64
	Revision   uint64
}

type Session struct {
	generation uint64
	sequence   uint64
	revision   uint64
	latest     map[string]uint64
}

func (s *Session) Reset()  { s.generation++; s.sequence = 0; s.revision = 0; s.latest = nil }
func (s *Session) Mutate() { s.revision++ }
func (s *Session) Begin(resource string) Token {
	s.sequence++
	if s.latest == nil {
		s.latest = map[string]uint64{}
	}
	s.latest[resource] = s.sequence
	return Token{s.generation, s.sequence, s.revision}
}
func (s *Session) Current(t Token) bool { return t.Generation == s.generation }
func (s *Session) Accept(resource string, t Token) bool {
	return s.Match(resource, t) && s.revision == t.Revision
}
func (s *Session) Generation() uint64 { return s.generation }
func (s *Session) Revision() uint64   { return s.revision }

// Job keeps timing policy common while domains retain their own partial-result rules.
type Job struct {
	Started, Finished time.Time
	Failures          int
	Busy              bool
}

func (j *Job) Complete(at time.Time, failed bool) {
	j.Finished, j.Busy = at, false
	if failed {
		j.Failures = min(j.Failures+1, 6)
	} else {
		j.Failures = 0
	}
}
func (j Job) Due(now time.Time, interval time.Duration) bool {
	if j.Busy {
		return false
	}
	delay := interval
	if j.Failures > 0 {
		delay = max(delay, min(30*time.Minute, time.Minute*time.Duration(1<<j.Failures)))
	}
	return now.Sub(maxTime(j.Started, j.Finished)) >= delay
}
func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func (s *Session) Match(resource string, t Token) bool {
	return s.Current(t) && s.latest[resource] == t.Sequence
}

// Scope cancels all work for an integration when its configuration changes.
// Tokens still gate completions because cancellation can race with a response.
type Scope struct {
	Session
	ctx    context.Context
	cancel context.CancelFunc
}

func (s *Scope) Configure(parent context.Context) {
	if s.cancel != nil {
		s.cancel()
	}
	s.Session.Reset()
	s.ctx, s.cancel = context.WithCancel(parent)
}
func (s *Scope) Context(parent context.Context) context.Context {
	if s.ctx == nil {
		return parent
	}
	return s.ctx
}
