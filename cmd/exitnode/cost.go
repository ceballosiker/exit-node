package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/iker/exit-node/internal/core"
)

func newCostCmd(opts *rootOpts) *cobra.Command {
	var period string
	cmd := &cobra.Command{
		Use:   "cost",
		Short: "Estimate spend for the active exit node",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			c, cleanup, err := opts.buildCore(ctx)
			if err != nil {
				return err
			}
			defer cleanup()

			var d time.Duration
			if period != "" {
				p, err := parsePeriod(period)
				if err != nil {
					return err
				}
				d = p
			}

			r, err := c.EstimateCost(ctx, core.CostOpts{Period: d})
			if err != nil {
				return fmt.Errorf("cost: %w", err)
			}
			if opts.json {
				return renderJSON(os.Stdout, r)
			}
			renderKV(os.Stdout, "machine_type", r.MachineType)
			renderKV(os.Stdout, "hours", fmt.Sprintf("%.2f", r.Hours))
			renderKV(os.Stdout, "usd_per_hour", fmt.Sprintf("%.4f", r.USDPerHour))
			renderKV(os.Stdout, "usd", fmt.Sprintf("%.2f", r.USD))
			if r.Note != "" {
				renderKV(os.Stdout, "note", r.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&period, "period", "", "Look-back window: 24h, 7d, month (default: since node creation)")
	return cmd
}

// parsePeriod accepts a few user-friendly aliases on top of Go's
// time.ParseDuration. "month" means 30 days; "Nd" means N days.
func parsePeriod(s string) (time.Duration, error) {
	if s == "month" {
		return 30 * 24 * time.Hour, nil
	}
	if len(s) > 1 && s[len(s)-1] == 'd' {
		var days int
		if _, err := fmt.Sscanf(s, "%dd", &days); err == nil {
			return time.Duration(days) * 24 * time.Hour, nil
		}
	}
	return time.ParseDuration(s)
}
