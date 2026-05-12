// Package verify implements the local-tailnet Probe interface by shelling
// out to `tailscale` and `curl`. The exec layer is abstracted as a
// commandRunner so tests can substitute a fake without touching real
// binaries.
package verify

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
)

// commandRunner abstracts process execution so tests can substitute a
// fake without touching real binaries. The production implementation
// (osCmdRunner) shells out via os/exec.
type commandRunner interface {
	Run(ctx context.Context, name string, args ...string) (stdout string, err error)
}

// osCmdRunner is the production runner. Its Run shells out to the given
// binary on PATH; stderr is folded into the returned error on non-zero
// exit so tests of error paths see the underlying message.
type osCmdRunner struct{}

func (osCmdRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return stdout.String(), errors.New(stderr.String())
		}
		return stdout.String(), err
	}
	return stdout.String(), nil
}
