package core

import (
	"context"
	"fmt"
)

// SyncPFSense pushes the active node's Tailscale IP to the configured
// pfSense gateway. Idempotent: if the gateway already matches, no write
// happens.
func (c *Core) SyncPFSense(ctx context.Context) error {
	active, err := c.store.GetActive()
	if err != nil {
		return fmt.Errorf("read state: %w", err)
	}
	if active == nil {
		return ErrNoActiveNode
	}
	gw, err := c.pf.GetGateway(ctx, c.cfg.PFSense.GatewayName)
	if err != nil {
		return fmt.Errorf("get gateway: %w", err)
	}
	if gw.IP == active.TailscaleIP {
		return nil
	}
	if err := c.pf.UpdateGatewayIP(ctx, c.cfg.PFSense.GatewayName, active.TailscaleIP); err != nil {
		return fmt.Errorf("update gateway: %w", err)
	}
	if err := c.pf.Apply(ctx); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	return nil
}
