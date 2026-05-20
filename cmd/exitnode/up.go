package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/iker/exit-node/internal/core"
)

func newUpCmd(opts *rootOpts) *cobra.Command {
	var (
		region      string
		machineType string
	)
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Provision an exit node (idempotent — returns the active one if it exists)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			c, cleanup, err := opts.buildCore(ctx)
			if err != nil {
				return err
			}
			defer cleanup()

			node, err := c.Up(ctx, core.UpOpts{Region: region, MachineType: machineType})
			if err != nil {
				return fmt.Errorf("up: %w", err)
			}
			return renderNode(os.Stdout, opts.json, node)
		},
	}
	cmd.Flags().StringVar(&region, "region", "", "GCP region (overrides config default)")
	cmd.Flags().StringVar(&machineType, "machine", "", "GCP machine type (overrides config default)")
	return cmd
}

func renderNode(w *os.File, asJSON bool, n any) error {
	if asJSON {
		return renderJSON(w, n)
	}
	// Type-assert to gcp.ExitNode is fine because callers pass that type.
	fmt.Fprintln(w, n)
	return nil
}
