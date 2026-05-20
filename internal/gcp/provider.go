package gcp

import "context"

// Provider abstracts the cloud backend that creates and manages exit-node
// VMs. Plan 1 only declares the interface; the concrete implementation
// lands in Plan 2.
type Provider interface {
	Provision(ctx context.Context, opts ProvisionOpts) (*ExitNode, error)
	Start(ctx context.Context, name string) error
	Stop(ctx context.Context, name string) error
	Destroy(ctx context.Context, name string) error
	List(ctx context.Context) ([]*ExitNode, error)
	// Get fetches a single managed instance by name.
	// Returns (nil, nil) if no instance with that name exists; any other
	// error (transport, permissions, etc.) is returned as (nil, err).
	Get(ctx context.Context, name string) (*ExitNode, error)
}
