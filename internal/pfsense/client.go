package pfsense

import "context"

// PFSenseClient abstracts the pfSense gateway management API.
type PFSenseClient interface {
	GetGateway(ctx context.Context, name string) (*Gateway, error)
	UpdateGatewayIP(ctx context.Context, name, ip string) error
	Apply(ctx context.Context) error
}
