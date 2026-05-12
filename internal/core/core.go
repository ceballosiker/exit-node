// Package core implements the orchestration logic for exitnode: rotate,
// up/down, status, health, sync, and cost. All external IO is abstracted
// through interfaces (gcp.Provider, tailscale.TailscaleClient,
// pfsense.PFSenseClient, verify.Probe) so this package can be exercised
// with hand-written mocks.
package core

import (
	"log/slog"

	"github.com/iker/exit-node/internal/config"
	"github.com/iker/exit-node/internal/gcp"
	"github.com/iker/exit-node/internal/pfsense"
	"github.com/iker/exit-node/internal/state"
	"github.com/iker/exit-node/internal/tailscale"
	"github.com/iker/exit-node/internal/verify"
)

// Core wires the orchestrator's dependencies.
type Core struct {
	cfg      *config.Config
	provider gcp.Provider
	ts       tailscale.TailscaleClient
	pf       pfsense.PFSenseClient
	probe    verify.Probe
	store    *state.Store
	log      *slog.Logger
}

// Deps groups the constructor inputs.
type Deps struct {
	Config   *config.Config
	Provider gcp.Provider
	TS       tailscale.TailscaleClient
	PF       pfsense.PFSenseClient
	Probe    verify.Probe
	Store    *state.Store
	Logger   *slog.Logger
}

// New constructs a Core. Logger defaults to slog.Default if nil.
func New(d Deps) *Core {
	log := d.Logger
	if log == nil {
		log = slog.Default()
	}
	return &Core{
		cfg:      d.Config,
		provider: d.Provider,
		ts:       d.TS,
		pf:       d.PF,
		probe:    d.Probe,
		store:    d.Store,
		log:      log,
	}
}
