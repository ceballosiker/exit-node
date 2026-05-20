// Package tailscale defines the Tailscale client interface and shared
// types. The concrete OAuth client + ephemeral key minting impl lands in
// Plan 2.
package tailscale

// Device is the canonical Tailscale device record.
type Device struct {
	ID          string
	Hostname    string
	TailscaleIP string
	Online      bool
	Tags        []string
}
