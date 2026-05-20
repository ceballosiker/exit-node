package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/iker/exit-node/internal/config"
	"github.com/iker/exit-node/internal/core"
	"github.com/iker/exit-node/internal/gcp"
	"github.com/iker/exit-node/internal/pfsense"
	"github.com/iker/exit-node/internal/state"
	"github.com/iker/exit-node/internal/tailscale"
	"github.com/iker/exit-node/internal/verify"
)

func defaultConfigPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "exitnode", "config.toml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "exitnode", "config.toml")
}

func defaultStatePath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "exitnode", "state.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "exitnode", "state.json")
}

// buildCore constructs a *core.Core with all live clients wired up. Each
// MCP tool call invokes this — opening + closing the state lock per
// invocation so the CLI and MCP can interleave without deadlocking each
// other.
func buildCore(ctx context.Context) (*core.Core, func(), error) {
	cfg, err := config.Load(defaultConfigPath())
	if err != nil {
		return nil, func() {}, fmt.Errorf("load config: %w", err)
	}

	tsID, tsSecret, err := cfg.ResolveTailscaleSecrets()
	if err != nil {
		return nil, func() {}, err
	}
	pfKey, err := cfg.ResolvePFSenseAPIKey()
	if err != nil {
		return nil, func() {}, err
	}
	_, gcpJSON, err := cfg.ResolveGCPCredentials()
	if err != nil {
		return nil, func() {}, err
	}

	provider, err := gcp.New(ctx, gcp.Options{
		Project:          cfg.GCP.Project,
		Region:           cfg.GCP.DefaultRegion,
		CredentialsJSON:  gcpJSON,
		Network:          cfg.GCP.Network,
		InstallScriptURL: cfg.Behavior.InstallScriptURL,
	})
	if err != nil {
		return nil, func() {}, fmt.Errorf("gcp: %w", err)
	}

	ts, err := tailscale.New(tailscale.Options{
		Tailnet:      cfg.Tailscale.Tailnet,
		ClientID:     tsID,
		ClientSecret: tsSecret,
	})
	if err != nil {
		return nil, func() {}, fmt.Errorf("tailscale: %w", err)
	}

	pf, err := pfsense.New(pfsense.Options{
		BaseURL:   cfg.PFSense.Host,
		APIKey:    pfKey,
		VerifyTLS: cfg.PFSense.VerifyTLS,
	})
	if err != nil {
		return nil, func() {}, fmt.Errorf("pfsense: %w", err)
	}

	probe := verify.New(cfg.Behavior.ProbeURL)

	store, err := state.Open(defaultStatePath())
	if err != nil {
		if errors.Is(err, state.ErrLocked) {
			return nil, func() {}, fmt.Errorf("state.json locked by another process")
		}
		return nil, func() {}, fmt.Errorf("state: %w", err)
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	c := core.New(core.Deps{
		Config: cfg, Provider: provider, TS: ts, PF: pf, Probe: probe, Store: store, Logger: log,
	})
	return c, func() { _ = store.Close() }, nil
}

