package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/iker/exit-node/internal/gcp"
)

// deviceLookupTimeout is the maximum time Destroy will wait for a Tailscale
// device to appear before giving up and proceeding with VM destruction.
const deviceLookupTimeout = 5 * time.Second

// ErrNameRequired is returned by Start when name is the empty string.
var ErrNameRequired = errors.New("core: name required")

// GetActive returns the currently-active exit node from the local state cache,
// or nil if none is recorded.
func (c *Core) GetActive() (*gcp.ExitNode, error) {
	return c.store.GetActive()
}

// Start calls the provider's Start method for the named exit node and then
// refreshes the local state cache if that node is currently the active one.
func (c *Core) Start(ctx context.Context, name string) error {
	if name == "" {
		return ErrNameRequired
	}
	if err := c.provider.Start(ctx, name); err != nil {
		return fmt.Errorf("provider.Start %s: %w", name, err)
	}
	return c.refreshIfActive(ctx, name)
}

// Stop stops a running exit-node VM by name. If the name matches the
// currently-active node, the on-disk state cache is refreshed.
func (c *Core) Stop(ctx context.Context, name string) error {
	if name == "" {
		return ErrNameRequired
	}
	if err := c.provider.Stop(ctx, name); err != nil {
		return fmt.Errorf("provider.Stop %s: %w", name, err)
	}
	return c.refreshIfActive(ctx, name)
}

// Destroy removes a named exit node: first it tries to look up and delete the
// Tailscale device, then it destroys the VM. If the VM destroy fails the error
// is returned. If the destroyed node was the active node, the state cache is
// cleared.
func (c *Core) Destroy(ctx context.Context, name string) error {
	if name == "" {
		return ErrNameRequired
	}

	// Step 1: Attempt to remove the Tailscale device.  A short timeout is used
	// so that a missing or already-deleted device does not block VM cleanup.
	dev, err := c.ts.WaitForDevice(ctx, name, deviceLookupTimeout)
	if err != nil {
		c.log.Warn("Destroy: could not look up TS device, skipping device delete", "name", name, "err", err)
	} else if dev != nil {
		if delErr := c.ts.DeleteDevice(ctx, dev.ID); delErr != nil {
			c.log.Warn("Destroy: ts.DeleteDevice failed, proceeding with VM destroy", "name", name, "deviceID", dev.ID, "err", delErr)
		}
	}

	// Step 2: Destroy the VM — load-bearing; failure aborts the operation.
	if err := c.provider.Destroy(ctx, name); err != nil {
		return fmt.Errorf("provider.Destroy %s: %w", name, err)
	}

	// Step 3: Clear state cache if this was the active node.
	active, err := c.store.GetActive()
	if err != nil {
		c.log.Warn("Destroy: could not read active state", "err", err)
		return nil
	}
	if active != nil && active.Name == name {
		if err := c.store.ClearActive(); err != nil {
			return fmt.Errorf("clear state after destroy: %w", err)
		}
	}
	return nil
}

// refreshIfActive re-fetches the named node from the provider and updates the
// local state cache — but only when name matches the currently-recorded active
// node. A provider Get failure is logged and swallowed; Start already succeeded
// so a state-refresh hiccup must not surface as a Start failure.
func (c *Core) refreshIfActive(ctx context.Context, name string) error {
	active, err := c.store.GetActive()
	if err != nil {
		c.log.Warn("refreshIfActive: could not read active state", "err", err)
		return nil
	}
	if active == nil || active.Name != name {
		return nil
	}
	fresh, err := c.provider.Get(ctx, name)
	if err != nil {
		c.log.Warn("refreshIfActive: provider.Get failed, skipping state update", "name", name, "err", err)
		return nil
	}
	if fresh == nil {
		c.log.Warn("refreshIfActive: provider.Get returned nil, skipping state update", "name", name)
		return nil
	}
	if err := c.store.SetActive(fresh); err != nil {
		c.log.Warn("refreshIfActive: SetActive failed", "name", name, "err", err)
	}
	return nil
}
