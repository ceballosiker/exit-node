package core

import (
	"context"
	"errors"
	"fmt"
)

// ErrNoActiveNode signals that a command needing an active node found none.
var ErrNoActiveNode = errors.New("no active exit node")

// DownOpts controls Down behavior.
type DownOpts struct {
	Destroy bool // if true, remove the VM + Tailscale device; else just Stop
}

// Down stops or destroys the currently active exit node.
func (c *Core) Down(ctx context.Context, opts DownOpts) error {
	active, err := c.store.GetActive()
	if err != nil {
		return fmt.Errorf("read state: %w", err)
	}
	if active == nil {
		return ErrNoActiveNode
	}
	if !opts.Destroy {
		if err := c.provider.Stop(ctx, active.Name); err != nil {
			return fmt.Errorf("stop %s: %w", active.Name, err)
		}
		return nil
	}
	if err := c.provider.Destroy(ctx, active.Name); err != nil {
		return fmt.Errorf("destroy %s: %w", active.Name, err)
	}
	if active.DeviceID != "" {
		if err := c.ts.DeleteDevice(ctx, active.DeviceID); err != nil {
			c.log.Warn("delete device failed (best-effort)", "err", err, "device_id", active.DeviceID)
		}
	}
	if err := c.store.ClearActive(); err != nil {
		c.log.Warn("clear state failed", "err", err)
	}
	return nil
}
