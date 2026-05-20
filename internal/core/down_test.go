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
)

func downFixture(t *testing.T, active *gcp.ExitNode) *rotateFixture {
	t.Helper()
	statePath := t.TempDir() + "/state.json"
	store, err := state.Open(statePath)
	if err != nil {
		t.Fatalf("state.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if active != nil {
		if err := store.SetActive(active); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	c := New(Deps{
		Config:   &config.Config{},
		Provider: newMockProvider(), TS: newMockTS(),
		PF: newMockPF(), Probe: &mockProbe{},
		Store: store, Logger: slog.New(slog.NewTextHandler(testWriter{t}, nil)),
	})
	return &rotateFixture{store: store, core: c,
		prov: c.provider.(*mockProvider), ts: c.ts.(*mockTS),
		pf: c.pf.(*mockPF), probe: c.probe.(*mockProbe)}
}

func TestDown_StopOnly(t *testing.T) {
	active := &gcp.ExitNode{Name: "vpn-1", DeviceID: "dev-1", State: gcp.StateRunning, CreatedAt: time.Now()}
	f := downFixture(t, active)
	if err := f.core.Down(context.Background(), DownOpts{Destroy: false}); err != nil {
		t.Fatalf("Down: %v", err)
	}
	if !hasCallWithArg(f.prov.calls, "Stop", "vpn-1") {
		t.Errorf("expected Stop(vpn-1); calls=%v", f.prov.calls)
	}
	if hasCallWithArg(f.prov.calls, "Destroy", "vpn-1") {
		t.Errorf("Destroy should not be called when Destroy=false")
	}
	// State cache retained.
	got, _ := f.store.GetActive()
	if got == nil || got.Name != "vpn-1" {
		t.Errorf("state cleared on Stop; got %v", got)
	}
}

func TestDown_Destroy(t *testing.T) {
	active := &gcp.ExitNode{Name: "vpn-1", DeviceID: "dev-1", State: gcp.StateRunning, CreatedAt: time.Now()}
	f := downFixture(t, active)
	if err := f.core.Down(context.Background(), DownOpts{Destroy: true}); err != nil {
		t.Fatalf("Down: %v", err)
	}
	if !hasCallWithArg(f.prov.calls, "Destroy", "vpn-1") {
		t.Errorf("expected Destroy(vpn-1); calls=%v", f.prov.calls)
	}
	if !hasCallWithArg(f.ts.calls, "DeleteDevice", "dev-1") {
		t.Errorf("expected DeleteDevice(dev-1); calls=%v", f.ts.calls)
	}
	// State cache cleared.
	got, _ := f.store.GetActive()
	if got != nil {
		t.Errorf("state not cleared; got %v", got)
	}
}

func TestDown_NoActive(t *testing.T) {
	f := downFixture(t, nil)
	err := f.core.Down(context.Background(), DownOpts{})
	if !errors.Is(err, ErrNoActiveNode) {
		t.Errorf("err = %v, want ErrNoActiveNode", err)
	}
}
