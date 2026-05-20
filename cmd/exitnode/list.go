package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newListCmd(opts *rootOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all exitnode-managed VMs in the GCP project",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			c, cleanup, err := opts.buildCore(ctx)
			if err != nil {
				return err
			}
			defer cleanup()

			nodes, err := c.List(ctx)
			if err != nil {
				return fmt.Errorf("list: %w", err)
			}
			if opts.json {
				return renderJSON(os.Stdout, nodes)
			}
			if len(nodes) == 0 {
				fmt.Fprintln(os.Stdout, "(no exitnode-managed VMs found)")
				return nil
			}
			fmt.Fprintf(os.Stdout, "%-32s %-14s %-12s %s\n", "NAME", "REGION", "STATE", "PUBLIC_IP")
			for _, n := range nodes {
				fmt.Fprintf(os.Stdout, "%-32s %-14s %-12v %s\n", n.Name, n.Region, n.State, n.PublicIP)
			}
			return nil
		},
	}
}
