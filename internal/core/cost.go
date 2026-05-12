package core

import (
	"context"
	"fmt"
	"time"
)

// CostOpts controls EstimateCost.
type CostOpts struct {
	// Period is the look-back window. If zero, defaults to time since CreatedAt.
	Period time.Duration
}

// CostResult is the estimate output.
type CostResult struct {
	MachineType string
	Hours       float64
	USDPerHour  float64
	USD         float64
	Note        string
}

// machineHourlyUSD is a static price table for the small set of machine
// types we support. Values from GCP's public on-demand list price (US
// regions) as of v0.1; intentionally not the Billing API.
var machineHourlyUSD = map[string]float64{
	"e2-micro":      0.008,
	"e2-small":      0.017,
	"e2-medium":     0.034,
	"n2-standard-2": 0.097,
}

// EstimateCost returns a rough cost estimate for the active exit node.
func (c *Core) EstimateCost(ctx context.Context, opts CostOpts) (*CostResult, error) {
	active, err := c.store.GetActive()
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	if active == nil {
		return nil, ErrNoActiveNode
	}
	rate, ok := machineHourlyUSD[active.MachineType]
	if !ok {
		return &CostResult{
			MachineType: active.MachineType,
			Note:        fmt.Sprintf("no static price for %q; configure manually", active.MachineType),
		}, nil
	}
	period := opts.Period
	if period == 0 {
		period = time.Since(active.CreatedAt)
	}
	hours := period.Hours()
	return &CostResult{
		MachineType: active.MachineType,
		Hours:       hours,
		USDPerHour:  rate,
		USD:         hours * rate,
		Note:        "list price, US regions, on-demand; ignores egress bandwidth and disk",
	}, nil
}
