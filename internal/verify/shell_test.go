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

func TestEgressVia_HappyPath_RestoresPrior(t *testing.T) {
	statusJSON := `{"ExitNodeStatus":{"ID":"prior-node-id"}}`
	fake := &fakeRunner{
		Responses: []fakeResponse{
			{Stdout: statusJSON, Err: nil},             // tailscale status --json
			{Stdout: "", Err: nil},                     // tailscale set --exit-node=<new>
			{Stdout: "pong\n", Err: nil},               // tailscale ping
			{Stdout: "203.0.113.7\n", Err: nil},        // curl
			{Stdout: "", Err: nil},                     // tailscale set --exit-node=prior-node-id (defer)
		},
	}
	p := &shellProbe{run: fake, probeURL: "https://example.com/ip"}

	got, err := p.EgressVia(context.Background(), "100.64.0.9")
	if err != nil {
		t.Fatalf("EgressVia: %v", err)
	}
	if got != "203.0.113.7" {
		t.Errorf("egress = %q, want 203.0.113.7", got)
	}

	if len(fake.calls) != 5 {
		t.Fatalf("expected 5 commands, got %d: %v", len(fake.calls), fake.lastNCommands(len(fake.calls)))
	}
	// The fifth call must be the restore to the prior ID.
	got5 := fake.calls[4]
	if got5.Name != "tailscale" || !contains(got5.Args, "--exit-node=prior-node-id") {
		t.Errorf("expected restore to prior id, got %s %v", got5.Name, got5.Args)
	}
}

// contains reports whether needle is in haystack.
func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
