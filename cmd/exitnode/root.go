package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/iker/exit-node/internal/config"
	"github.com/iker/exit-node/internal/core"
	"github.com/iker/exit-node/internal/gcp"
	"github.com/iker/exit-node/internal/pfsense"
	"github.com/iker/exit-node/internal/state"
	"github.com/iker/exit-node/internal/tailscale"
	"github.com/iker/exit-node/internal/verify"
)

// rootOpts collects the global flags. One instance per process; subcommands
// read from it via closure.
type rootOpts struct {
	configPath string
	json       bool
	verbose    bool
}

func newRootCmd() *cobra.Command {
	opts := &rootOpts{}

	cmd := &cobra.Command{
		Use:           "exitnode",
		Short:         "On-demand Tailscale exit nodes that rotate across cloud regions",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.PersistentFlags().StringVar(&opts.configPath, "config", defaultConfigPath(), "Path to config.toml")
	cmd.PersistentFlags().BoolVar(&opts.json, "json", false, "Machine-readable JSON output")
	cmd.PersistentFlags().BoolVarP(&opts.verbose, "verbose", "v", false, "Verbose (debug) logging")

	cmd.AddCommand(
		newUpCmd(opts),
		newDownCmd(opts),
		newRotateCmd(opts),
		newListCmd(opts),
		newStatusCmd(opts),
		newHealthCmd(opts),
		newPFSenseCmd(opts),
		newCostCmd(opts),
	)
	return cmd
}

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

// Subcommand stubs (real impls in subsequent tasks). Kept visible (not
// Hidden) so the help-output smoke test in Step 7 passes even before
// the real impls land.
func newRotateCmd(*rootOpts) *cobra.Command  { return &cobra.Command{Use: "rotate"} }
func newListCmd(*rootOpts) *cobra.Command    { return &cobra.Command{Use: "list"} }
func newStatusCmd(*rootOpts) *cobra.Command  { return &cobra.Command{Use: "status"} }
func newHealthCmd(*rootOpts) *cobra.Command  { return &cobra.Command{Use: "health"} }
func newPFSenseCmd(*rootOpts) *cobra.Command { return &cobra.Command{Use: "pfsense"} }
func newCostCmd(*rootOpts) *cobra.Command    { return &cobra.Command{Use: "cost"} }

// buildCore loads config + secrets, opens the state store, and constructs
// a *core.Core with real client implementations wired in. Returned cleanup
// must always be called (releases the state lock).
func (o *rootOpts) buildCore(ctx context.Context) (*core.Core, func(), error) {
	cfg, err := config.Load(o.configPath)
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
		return nil, func() {}, fmt.Errorf("gcp client: %w", err)
	}

	ts, err := tailscale.New(tailscale.Options{
		Tailnet:      cfg.Tailscale.Tailnet,
		ClientID:     tsID,
		ClientSecret: tsSecret,
	})
	if err != nil {
		return nil, func() {}, fmt.Errorf("tailscale client: %w", err)
	}

	pf, err := pfsense.New(pfsense.Options{
		BaseURL:   cfg.PFSense.Host,
		APIKey:    pfKey,
		VerifyTLS: cfg.PFSense.VerifyTLS,
	})
	if err != nil {
		return nil, func() {}, fmt.Errorf("pfsense client: %w", err)
	}

	probe := verify.New(cfg.Behavior.ProbeURL)

	store, err := state.Open(defaultStatePath())
	if err != nil {
		if errors.Is(err, state.ErrLocked) {
			return nil, func() {}, fmt.Errorf("another exitnode process is running (state.json locked)")
		}
		return nil, func() {}, fmt.Errorf("open state: %w", err)
	}

	level := slog.LevelInfo
	if o.verbose {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	c := core.New(core.Deps{
		Config: cfg, Provider: provider, TS: ts, PF: pf, Probe: probe, Store: store, Logger: log,
	})
	return c, func() { _ = store.Close() }, nil
}
