package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/iker/exit-node/internal/core"
)

func newDownCmd(opts *rootOpts) *cobra.Command {
	var destroy bool
	cmd := &cobra.Command{
		Use:   "down",
		Short: "Stop (or with --destroy, terminate) the currently active exit node",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			c, cleanup, err := opts.buildCore(ctx)
			if err != nil {
				return err
			}
			defer cleanup()
			if err := c.Down(ctx, core.DownOpts{Destroy: destroy}); err != nil {
				return fmt.Errorf("down: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&destroy, "destroy", false, "Destroy the VM + tailscale device (default: stop only)")
	return cmd
}
