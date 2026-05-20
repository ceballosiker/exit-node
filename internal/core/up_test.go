package core

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/iker/exit-node/internal/config"
	"github.com/iker/exit-node/internal/gcp"
	"github.com/iker/exit-node/internal/state"
	"github.com/iker/exit-node/internal/tailscale"
)

func upFixture(t *testing.T) *rotateFixture {
	t.Helper()
	statePath := t.TempDir() + "/state.json"
	store, err := state.Open(statePath)
	if err != nil {
		t.Fatalf("state.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	newN := &gcp.ExitNode{
		Name: "vpn-up-1", Region: "us-west1", Zone: "us-west1-a",
		MachineType: "e2-micro", PublicIP: "35.5.5.5",
		State: gcp.StateRunning, CreatedAt: time.Now(),
	}
	dev := &tailscale.Device{ID: "dev-up", Hostname: "vpn-up-1", TailscaleIP: "100.64.0.55"}

	cfg := &config.Config{
		GCP: config.GCPConfig{
			Project: "p", DefaultRegion: "us-west1", DefaultMachineType: "e2-micro",
			Network: "default", DiskSizeGB: 10,
		},
		Tailscale: config.TailscaleConfig{Tags: []string{"tag:exit-node"}, EphemeralKeyTTL: 5 * time.Minute},
		Behavior: config.BehaviorConfig{
			VerifyPreCutover:    true,
			ProbeURL:            "https://example.com/ip",
			RegistrationTimeout: 90 * time.Second,
			InstallScriptURL:    "https://example.com/install.sh",
		},
	}
	prov := newMockProvider()
	prov.ProvisionResult = newN
	ts := newMockTS()
	ts.WaitDevice = dev
	probe := &mockProbe{EgressViaResult: newN.PublicIP}
	pf := newMockPF()
	c := New(Deps{
		Config: cfg, Provider: prov, TS: ts, PF: pf, Probe: probe,
		Store: store, Logger: slog.New(slog.NewTextHandler(testWriter{t}, nil)),
	})
	return &rotateFixture{
		cfg: cfg, prov: prov, ts: ts, pf: pf, probe: probe, store: store, core: c,
		newNode: newN, newDev: dev,
	}
}

func TestUp_CreatesWhenNoActive(t *testing.T) {
	f := upFixture(t)
	got, err := f.core.Up(context.Background(), UpOpts{})
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	if got == nil || got.Name != f.newNode.Name {
		t.Errorf("got %v, want %s", got, f.newNode.Name)
	}
	if !hasCallWithArg(f.prov.calls, "Provision", "") && len(f.prov.names()) == 0 {
		t.Errorf("Provision was not called; calls=%v", f.prov.calls)
	}
}

func TestUp_NoopWhenActiveExists(t *testing.T) {
	f := upFixture(t)
	existing := &gcp.ExitNode{Name: "already-up", State: gcp.StateRunning}
	if err := f.store.SetActive(existing); err != nil {
		t.Fatalf("seed: %v", err)
	}
	got, err := f.core.Up(context.Background(), UpOpts{})
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	if got.Name != "already-up" {
		t.Errorf("got %v, want already-up", got)
	}
	if len(f.prov.names()) != 0 {
		t.Errorf("expected no provider calls; got %v", f.prov.names())
	}
}

var _ = errors.New
