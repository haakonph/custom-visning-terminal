package syncstate

import (
	"testing"
	"time"
)

func TestRequestGenerationSequenceAndRevision(t *testing.T) {
	s := Session{}
	s.Reset()
	old := s.Begin("query")
	current := s.Begin("query")
	if s.Accept("query", old) || !s.Accept("query", current) {
		t.Fatal("request sequence ignored")
	}
	s.Mutate()
	if s.Accept("query", current) || !s.Match("query", current) {
		t.Fatal("read revision confused with action identity")
	}
	s.Reset()
	if s.Current(current) {
		t.Fatal("old session accepted")
	}
}
func TestSchedulingBackoffUsesSuppliedTime(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	job := Job{Started: now, Busy: true}
	if job.Due(now.Add(time.Hour), time.Minute) {
		t.Fatal("busy job duplicated")
	}
	job.Complete(now, true)
	if job.Due(now.Add(time.Minute), time.Minute) || !job.Due(now.Add(2*time.Minute), time.Minute) {
		t.Fatal("failure backoff")
	}
	job.Complete(now.Add(2*time.Minute), false)
	if !job.Due(now.Add(3*time.Minute), time.Minute) || job.Failures != 0 {
		t.Fatal("successful recovery")
	}
}
