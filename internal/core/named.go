package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/iker/exit-node/internal/gcp"
)

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
