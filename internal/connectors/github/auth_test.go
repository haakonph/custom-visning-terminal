package github

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestResolveToken(t *testing.T) {
	tests := []struct {
		name, env, cli string
		err            error
		want           string
		wantErr        bool
		calls          int
	}{
		{name: "environment wins", env: " env-token \n", cli: "cli-token", want: "env-token", calls: 0},
		{name: "CLI fallback", cli: " cli-token\n", want: "cli-token", calls: 1},
		{name: "missing CLI", err: errors.New("secret stderr"), wantErr: true, calls: 1},
		{name: "empty CLI", wantErr: true, calls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			token, err := ResolveToken(context.Background(), func(string) string { return tt.env }, func(context.Context) ([]byte, error) { calls++; return []byte(tt.cli), tt.err })
			if token != tt.want || (err != nil) != tt.wantErr || calls != tt.calls {
				t.Fatalf("token=%q err=%v calls=%d", token, err, calls)
			}
			if err != nil && strings.Contains(err.Error(), "secret stderr") {
				t.Fatal("leaked CLI stderr")
			}
		})
	}
}
