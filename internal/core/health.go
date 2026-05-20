package core

import (
	"context"
	"fmt"
)

// HealthResult is the result of a Health probe.
type HealthResult struct {
	OK         bool
	EgressIP   string // observed
	ExpectedIP string // GCP public IP of the active node
	ProbeErr   error  // non-nil if probe itself errored
}

// Health runs the pre-cutover probe against the currently active node
// without mutating anything else.
func (c *Core) Health(ctx context.Context) (*HealthResult, error) {
	active, err := c.store.GetActive()
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	if active == nil {
		return nil, ErrNoActiveNode
	}
	egress, perr := c.probe.EgressVia(ctx, active.TailscaleIP)
	return &HealthResult{
		OK:         perr == nil && egress == active.PublicIP,
		EgressIP:   egress,
		ExpectedIP: active.PublicIP,
		ProbeErr:   perr,
	}, nil
}
