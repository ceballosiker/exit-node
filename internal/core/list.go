package core

import (
	"context"
	"fmt"

	"github.com/iker/exit-node/internal/gcp"
)

// List returns all managed exit nodes.
func (c *Core) List(ctx context.Context) ([]*gcp.ExitNode, error) {
	nodes, err := c.provider.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	return nodes, nil
}
