package core

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/iker/exit-node/internal/config"
	"github.com/iker/exit-node/internal/gcp"
	"github.com/iker/exit-node/internal/state"
)

type namedFixtureT struct {
	c     *Core
	store *state.Store
	prov  *mockProvider
	ts    *mockTS
}

func namedFixture(t *testing.T, active *gcp.ExitNode) *namedFixtureT {
	t.Helper()
	statePath := t.TempDir() + "/state.json"
	store, err := state.Open(statePath)
	if err != nil {
		t.Fatalf("state.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if active != nil {
		if err := store.SetActive(active); err != nil {
			t.Fatalf("SetActive: %v", err)
		}
	}
	cfg := &config.Config{}
	prov := newMockProvider()
	ts := newMockTS()
	pf := newMockPF()
	probe := &mockProbe{}
	c := New(Deps{
		Config: cfg, Provider: prov, TS: ts, PF: pf, Probe: probe,
		Store: store, Logger: slog.New(slog.NewTextHandler(testWriter{t}, nil)),
	})
	return &namedFixtureT{c: c, store: store, prov: prov, ts: ts}
}

func TestStart_CallsProviderStart(t *testing.T) {
	f := namedFixture(t, nil)

	if err := f.c.Start(context.Background(), "vpn-us-central1-abc"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !hasCallWithArg(f.prov.calls, "Start", "vpn-us-central1-abc") {
		t.Errorf("provider.Start not called with expected name; calls=%v", f.prov.calls)
	}
}

func TestStart_RefreshesStateWhenNameMatchesActive(t *testing.T) {
	active := &gcp.ExitNode{Name: "vpn-us-central1-abc", State: gcp.StateStopped}
	f := namedFixture(t, active)
	f.prov.GetResult["vpn-us-central1-abc"] = &gcp.ExitNode{
		Name: "vpn-us-central1-abc", State: gcp.StateRunning, PublicIP: "1.2.3.4",
	}

	if err := f.c.Start(context.Background(), "vpn-us-central1-abc"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	got, err := f.store.GetActive()
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	if got == nil {
		t.Fatal("GetActive returned nil, want refreshed node")
	}
	if got.State != gcp.StateRunning || got.PublicIP != "1.2.3.4" {
		t.Errorf("state not refreshed: got State=%v PublicIP=%q", got.State, got.PublicIP)
	}
}

func TestStart_DoesNotTouchStateWhenNameMismatch(t *testing.T) {
	active := &gcp.ExitNode{Name: "vpn-us-central1-abc", State: gcp.StateRunning, PublicIP: "9.9.9.9"}
	f := namedFixture(t, active)
	// Provider Get for the different name returns nothing (default empty map).

	if err := f.c.Start(context.Background(), "vpn-eu-west1-xyz"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	got, err := f.store.GetActive()
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	if got == nil || got.PublicIP != "9.9.9.9" {
		t.Errorf("state mutated unexpectedly: %+v", got)
	}
}

func TestStart_RejectsEmptyName(t *testing.T) {
	f := namedFixture(t, nil)
	err := f.c.Start(context.Background(), "")
	if !errors.Is(err, ErrNameRequired) {
		t.Errorf("got %v, want ErrNameRequired", err)
	}
	// Provider.Start must not have been called.
	if hasCallWithArg(f.prov.calls, "Start", "") {
		t.Errorf("provider.Start was called despite empty name")
	}
}
