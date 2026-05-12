package core

import (
	"context"
	"fmt"

	"github.com/iker/exit-node/internal/gcp"
)

// StatusResult is the result of a Status call.
type StatusResult struct {
	Cached *gcp.ExitNode // from state.json; nil if no active
	Live   *gcp.ExitNode // freshly fetched from the provider; nil if Cached is nil
}

// Drift reports whether the live state differs from the cached state.
func (s *StatusResult) Drift() bool {
	if s.Cached == nil || s.Live == nil {
		return s.Cached != s.Live // both nil = no drift
	}
	return s.Cached.State != s.Live.State || s.Cached.PublicIP != s.Live.PublicIP
}

// Status fetches the cached active node and the live provider record.
func (c *Core) Status(ctx context.Context) (*StatusResult, error) {
	cached, err := c.store.GetActive()
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	out := &StatusResult{Cached: cached}
	if cached != nil {
		live, err := c.provider.Get(ctx, cached.Name)
		if err != nil {
			return nil, fmt.Errorf("get live: %w", err)
		}
		out.Live = live
	}
	return out, nil
}
