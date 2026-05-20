package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func newPFSenseCmd(opts *rootOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pfsense",
		Short: "pfSense gateway operations",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "sync",
		Short: "Push the active node's Tailscale IP to the configured pfSense gateway",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			c, cleanup, err := opts.buildCore(ctx)
			if err != nil {
				return err
			}
			defer cleanup()
			if err := c.SyncPFSense(ctx); err != nil {
				return fmt.Errorf("pfsense sync: %w", err)
			}
			return nil
		},
	})
	return cmd
}
