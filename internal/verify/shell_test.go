package verify

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeRunner records every command and returns canned outputs / errors
// per call. Tests configure Responses in order; the i-th Run call
// returns Responses[i].
type fakeRunner struct {
	calls     []fakeCall
	Responses []fakeResponse
}

type fakeCall struct {
	Name string
	Args []string
}

type fakeResponse struct {
	Stdout string
	Err    error
}

func (f *fakeRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	i := len(f.calls)
	f.calls = append(f.calls, fakeCall{Name: name, Args: args})
	if i >= len(f.Responses) {
		return "", errors.New("fakeRunner: unexpected extra call")
	}
	r := f.Responses[i]
	return r.Stdout, r.Err
}

func (f *fakeRunner) lastNCommands(n int) []string {
	out := make([]string, 0, n)
	start := len(f.calls) - n
	if start < 0 {
		start = 0
	}
	for _, c := range f.calls[start:] {
		out = append(out, c.Name+" "+strings.Join(c.Args, " "))
	}
	return out
}

func TestShellRunnerInterface(t *testing.T) {
	// Sentinel test — confirms commandRunner is the seam and *fakeRunner
	// implements it. Compile-only assertion.
	var _ commandRunner = (*fakeRunner)(nil)
}
