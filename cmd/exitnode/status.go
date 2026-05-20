package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newStatusCmd(opts *rootOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the currently active exit node and any state drift",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			c, cleanup, err := opts.buildCore(ctx)
			if err != nil {
				return err
			}
			defer cleanup()

			s, err := c.Status(ctx)
			if err != nil {
				return fmt.Errorf("status: %w", err)
			}
			if opts.json {
				return renderJSON(os.Stdout, s)
			}
			if s.Cached == nil {
				fmt.Fprintln(os.Stdout, "no active exit node")
				return nil
			}
			renderKV(os.Stdout, "name", s.Cached.Name)
			renderKV(os.Stdout, "region", s.Cached.Region)
			if s.Live != nil {
				renderKV(os.Stdout, "live_state", s.Live.State)
				renderKV(os.Stdout, "public_ip", s.Live.PublicIP)
			}
			if s.Drift() {
				fmt.Fprintln(os.Stdout, "(state drift detected — run `exitnode list` to inspect)")
			}
			return nil
		},
	}
}
