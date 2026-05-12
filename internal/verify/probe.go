// Package verify defines the Probe interface. The concrete shellout impl
// (sets tailscale exit-node, curls, restores) lands in Plan 2.
package verify

import "context"

// Probe verifies egress routing from the orchestrator host.
type Probe interface {
	// EgressVia temporarily routes the orchestrator host's egress through
	// the given tailscale IP, curls an IP-echo service, restores the prior
	// exit-node setting, and returns the observed egress IP. The restore
	// runs in a defer and executes even on probe failure.
	EgressVia(ctx context.Context, tailscaleIP string) (egressIP string, err error)

	// EgressDirect curls the IP-echo service with no tailscale exit-node
	// override (or temporarily clears it), then restores. Used for the
	// post-cutover check that traffic flows via the orchestrator's default
	// route (LAN → pfSense → new exit node).
	EgressDirect(ctx context.Context) (egressIP string, err error)
}
