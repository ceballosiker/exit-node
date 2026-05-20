package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newHealthCmd(opts *rootOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Probe egress through the active exit node",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			c, cleanup, err := opts.buildCore(ctx)
			if err != nil {
				return err
			}
			defer cleanup()

			h, err := c.Health(ctx)
			if err != nil {
				return fmt.Errorf("health: %w", err)
			}
			if opts.json {
				return renderJSON(os.Stdout, h)
			}
			renderKV(os.Stdout, "ok", h.OK)
			renderKV(os.Stdout, "egress_ip", h.EgressIP)
			renderKV(os.Stdout, "expected_ip", h.ExpectedIP)
			if h.ProbeErr != nil {
				renderKV(os.Stdout, "probe_err", h.ProbeErr.Error())
			}
			if !h.OK {
				os.Exit(2)
			}
			return nil
		},
	}
}
