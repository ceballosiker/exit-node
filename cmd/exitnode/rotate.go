package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/iker/exit-node/internal/core"
)

func newRotateCmd(opts *rootOpts) *cobra.Command {
	var region string
	cmd := &cobra.Command{
		Use:   "rotate",
		Short: "Provision a new exit node, cut pfSense over, destroy the old one",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			c, cleanup, err := opts.buildCore(ctx)
			if err != nil {
				return err
			}
			defer cleanup()

			res, err := c.Rotate(ctx, core.RotateOpts{Region: region})
			if err != nil {
				return fmt.Errorf("rotate: %w", err)
			}
			if opts.json {
				return renderJSON(os.Stdout, res)
			}
			fmt.Fprintln(os.Stdout, "rotated:")
			if res.Old != nil {
				renderKV(os.Stdout, "old", res.Old.Name)
			}
			if res.New != nil {
				renderKV(os.Stdout, "new", res.New.Name)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&region, "region", "", "GCP region for the new node (overrides config default)")
	return cmd
}
