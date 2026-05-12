package core

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/iker/exit-node/internal/config"
	"github.com/iker/exit-node/internal/gcp"
	"github.com/iker/exit-node/internal/state"
	"github.com/iker/exit-node/internal/tailscale"
)

// rotateFixture wires a Core with all mocks pre-populated for a happy
// rotate (no pfSense sync by default; tests override per case).
type rotateFixture struct {
	cfg   *config.Config
	prov  *mockProvider
	ts    *mockTS
	pf    *mockPF
	probe *mockProbe
	store *state.Store
	core  *Core
	clock *clock

	oldNode *gcp.ExitNode
	newNode *gcp.ExitNode
	newDev  *tailscale.Device
}

// orderedNames returns the names of every recorded call across all mocks
// in chronological order (using the shared sequence clock).
func (f *rotateFixture) orderedNames() []string {
	all := append([]callRecord{}, f.ts.calls...)
	all = append(all, f.prov.calls...)
	all = append(all, f.pf.calls...)
	all = append(all, f.probe.calls...)
	sort.Slice(all, func(i, j int) bool { return all[i].Seq < all[j].Seq })
	out := make([]string, len(all))
	for i, c := range all {
		out[i] = c.Name
	}
	return out
}

func newRotateFixture(t *testing.T) *rotateFixture {
	t.Helper()
	statePath := t.TempDir() + "/state.json"
	store, err := state.Open(statePath)
	if err != nil {
		t.Fatalf("state.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	old := &gcp.ExitNode{
		Name: "vpn-old", Region: "us-west1", Zone: "us-west1-a",
		MachineType: "e2-micro", PublicIP: "35.0.0.1", TailscaleIP: "100.64.0.1",
		DeviceID: "dev-old", State: gcp.StateRunning,
		CreatedAt: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := store.SetActive(old); err != nil {
		t.Fatalf("seed: %v", err)
	}

	newN := &gcp.ExitNode{
		Name: "vpn-new", Region: "asia-southeast1", Zone: "asia-southeast1-a",
		MachineType: "e2-micro", PublicIP: "35.9.9.9",
		State: gcp.StateRunning, CreatedAt: time.Now(),
	}
	dev := &tailscale.Device{ID: "dev-new", Hostname: "vpn-new", TailscaleIP: "100.64.0.9", Online: true}

	cfg := &config.Config{
		GCP: config.GCPConfig{
			Project: "p", DefaultRegion: "us-west1", DefaultMachineType: "e2-micro",
			Network: "default", DiskSizeGB: 10,
		},
		Tailscale: config.TailscaleConfig{Tailnet: "x.com", Tags: []string{"tag:exit-node"}, EphemeralKeyTTL: 5 * time.Minute},
		PFSense:   config.PFSenseConfig{GatewayName: "GW"},
		Behavior: config.BehaviorConfig{
			AutoSyncPFSense:     false, // happy path without pfSense
			VerifyPreCutover:    true,
			VerifyPostCutover:   false,
			ProbeURL:            "https://example.com/ip",
			RegistrationTimeout: 90 * time.Second,
			InstallScriptURL:    "https://example.com/install.sh",
		},
	}

	clk := &clock{}

	prov := newMockProvider()
	prov.ProvisionResult = newN
	prov.GetResult[old.Name] = old
	prov.attachClock(clk)

	ts := newMockTS()
	ts.WaitDevice = dev
	ts.attachClock(clk)

	pf := newMockPF()
	pf.Gateways["GW"] = "100.64.0.1"
	pf.attachClock(clk)

	probe := &mockProbe{EgressViaResult: newN.PublicIP}
	probe.attachClock(clk)

	c := New(Deps{
		Config: cfg, Provider: prov, TS: ts, PF: pf, Probe: probe,
		Store: store, Logger: slog.New(slog.NewTextHandler(testWriter{t}, nil)),
	})

	return &rotateFixture{
		cfg: cfg, prov: prov, ts: ts, pf: pf, probe: probe,
		store: store, core: c, clock: clk,
		oldNode: old, newNode: newN, newDev: dev,
	}
}

// testWriter forwards slog output to t.Logf so tests stay readable.
type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) { w.t.Logf("%s", p); return len(p), nil }

func TestRotateHappyPath_NoPFSense(t *testing.T) {
	f := newRotateFixture(t)
	res, err := f.core.Rotate(context.Background(), RotateOpts{Region: "asia-southeast1"})
	if err != nil {
		t.Fatalf("Rotate err = %v", err)
	}
	if res.New == nil || res.New.Name != f.newNode.Name {
		t.Errorf("res.New.Name = %v, want %s", res.New, f.newNode.Name)
	}
	if res.Old == nil || res.Old.Name != f.oldNode.Name {
		t.Errorf("res.Old.Name = %v, want %s", res.Old, f.oldNode.Name)
	}

	wantCalls := []string{
		"MintEphemeralAuthKey",
		"Provision",
		"WaitForDevice",
		"AuthorizeExitNode",
		"SetTags",
		"EgressVia",
	}
	if !reflect.DeepEqual(f.ts.names()[:1], wantCalls[:1]) {
		t.Errorf("first call: %v", f.ts.names())
	}
	// Chronological sequence across all mocks (uses shared clock):
	combined := f.orderedNames()
	if !containsAll(combined, wantCalls...) {
		t.Errorf("missing expected calls; got combined=%v", combined)
	}

	// pfSense should NOT have been touched.
	if len(f.pf.names()) != 0 {
		t.Errorf("expected no pfSense calls, got %v", f.pf.names())
	}

	// State cache updated to new node.
	got, err := f.store.GetActive()
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	if got == nil || got.Name != f.newNode.Name {
		t.Errorf("state.Active = %v, want %s", got, f.newNode.Name)
	}

	// Old node should be destroyed (best-effort, but called).
	if !hasCallWithArg(f.prov.calls, "Destroy", f.oldNode.Name) {
		t.Errorf("expected Destroy(%s) call, got %v", f.oldNode.Name, f.prov.calls)
	}
	if !hasCallWithArg(f.ts.calls, "DeleteDevice", f.oldNode.DeviceID) {
		t.Errorf("expected DeleteDevice(%s) call, got %v", f.oldNode.DeviceID, f.ts.calls)
	}

	// New node should NOT have been destroyed.
	if hasCallWithArg(f.prov.calls, "Destroy", f.newNode.Name) {
		t.Errorf("new node was destroyed on happy path; calls=%v", f.prov.calls)
	}
}

func containsAll(haystack []string, needles ...string) bool {
	idx := 0
	for _, h := range haystack {
		if idx < len(needles) && h == needles[idx] {
			idx++
		}
	}
	return idx == len(needles)
}

func hasCallWithArg(calls []callRecord, name string, arg any) bool {
	for _, c := range calls {
		if c.Name != name {
			continue
		}
		for _, a := range c.Args {
			if reflect.DeepEqual(a, arg) {
				return true
			}
		}
	}
	return false
}

// Sentinel to prevent "imported and not used" if errors becomes unused later.
var _ = errors.New

func TestRotateHappyPath_WithPFSense(t *testing.T) {
	f := newRotateFixture(t)
	f.cfg.Behavior.AutoSyncPFSense = true

	res, err := f.core.Rotate(context.Background(), RotateOpts{Region: "asia-southeast1"})
	if err != nil {
		t.Fatalf("Rotate err = %v", err)
	}
	if res.New == nil || res.New.Name != f.newNode.Name {
		t.Errorf("res.New = %v", res.New)
	}

	wantPFCalls := []string{"GetGateway", "UpdateGatewayIP", "Apply"}
	if !reflect.DeepEqual(f.pf.names(), wantPFCalls) {
		t.Errorf("pfSense calls = %v, want %v", f.pf.names(), wantPFCalls)
	}
	if got := f.pf.Gateways["GW"]; got != f.newDev.TailscaleIP {
		t.Errorf("gateway IP = %s, want %s", got, f.newDev.TailscaleIP)
	}
}
