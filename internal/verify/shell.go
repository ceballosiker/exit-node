// Package verify implements the local-tailnet Probe interface by shelling
// out to `tailscale` and `curl`. The exec layer is abstracted as a
// commandRunner so tests can substitute a fake without touching real
// binaries.
package verify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
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

// New constructs the production Probe. The return type is *shellProbe in
// this task because EgressDirect (required by the Probe interface) is
// added in Task 5; Task 6 will widen this to Probe alongside the
// compile-time interface sentinel.
func New(probeURL string) *shellProbe {
	return &shellProbe{run: osCmdRunner{}, probeURL: probeURL}
}

// shellProbe is the concrete Probe implementation. It is unexported; the
// returned interface is the only public surface.
type shellProbe struct {
	run      commandRunner
	probeURL string
}

// EgressVia routes the orchestrator host's egress through the given
// Tailscale IP, curls the configured probe URL, restores the prior
// exit-node setting (always, even on probe failure), and returns the
// observed egress IP.
func (p *shellProbe) EgressVia(ctx context.Context, tailscaleIP string) (string, error) {
	priorID, err := p.captureExitNodeID(ctx)
	if err != nil {
		return "", fmt.Errorf("capture prior exit-node: %w", err)
	}
	// Restore runs in a defer so it executes on ANY return path below.
	defer p.restoreExitNode(priorID)

	if _, err := p.run.Run(ctx, "tailscale", "set",
		fmt.Sprintf("--exit-node=%s", tailscaleIP),
		"--exit-node-allow-lan-access=true",
	); err != nil {
		return "", fmt.Errorf("set exit-node: %w", err)
	}
	if _, err := p.run.Run(ctx, "tailscale", "ping", "--timeout=10s", tailscaleIP); err != nil {
		return "", fmt.Errorf("tailscale ping: %w", err)
	}
	out, err := p.run.Run(ctx, "curl", "--silent", "--max-time", "10", p.probeURL)
	if err != nil {
		return "", fmt.Errorf("curl probe URL: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// captureExitNodeID parses `tailscale status --json` and returns the
// current ExitNodeStatus.ID (empty string if no exit node was set).
func (p *shellProbe) captureExitNodeID(ctx context.Context) (string, error) {
	out, err := p.run.Run(ctx, "tailscale", "status", "--json")
	if err != nil {
		return "", err
	}
	var parsed struct {
		ExitNodeStatus *struct {
			ID string `json:"ID"`
		} `json:"ExitNodeStatus"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		return "", fmt.Errorf("parse tailscale status json: %w", err)
	}
	if parsed.ExitNodeStatus == nil {
		return "", nil
	}
	return parsed.ExitNodeStatus.ID, nil
}

// EgressDirect curls the probe URL with no Tailscale exit-node override,
// then restores the prior exit-node setting. Used for the post-cutover
// check that traffic flows via the host's default route (LAN → pfSense →
// new exit node).
func (p *shellProbe) EgressDirect(ctx context.Context) (string, error) {
	priorID, err := p.captureExitNodeID(ctx)
	if err != nil {
		return "", fmt.Errorf("capture prior exit-node: %w", err)
	}
	defer p.restoreExitNode(priorID)

	if _, err := p.run.Run(ctx, "tailscale", "set", "--exit-node="); err != nil {
		return "", fmt.Errorf("clear exit-node: %w", err)
	}
	out, err := p.run.Run(ctx, "curl", "--silent", "--max-time", "10", p.probeURL)
	if err != nil {
		return "", fmt.Errorf("curl probe URL: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// restoreExitNode is best-effort; errors are intentionally swallowed
// because we're already in a defer chain and the caller has its own
// error to return. A failed restore is loud at the host level (you'll
// notice your egress is wrong); this is documented in the README's
// troubleshooting section.
func (p *shellProbe) restoreExitNode(priorID string) {
	// Use a fresh context: the request ctx may already be canceled.
	ctx := context.Background()
	arg := "--exit-node="
	if priorID != "" {
		arg = "--exit-node=" + priorID
	}
	_, _ = p.run.Run(ctx, "tailscale", "set", arg)
}
