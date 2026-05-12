package core

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"testing"

	"github.com/iker/exit-node/internal/config"
	"github.com/iker/exit-node/internal/gcp"
	"github.com/iker/exit-node/internal/state"
)

func syncFixture(t *testing.T, active *gcp.ExitNode, gwIP string) *rotateFixture {
	t.Helper()
	statePath := t.TempDir() + "/state.json"
	store, err := state.Open(statePath)
	if err != nil {
		t.Fatalf("state.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if active != nil {
		_ = store.SetActive(active)
	}
	pf := newMockPF()
	pf.Gateways["GW"] = gwIP
	c := New(Deps{
		Config: &config.Config{PFSense: config.PFSenseConfig{GatewayName: "GW"}},
		Provider: newMockProvider(), TS: newMockTS(),
		PF: pf, Probe: &mockProbe{},
		Store: store, Logger: slog.New(slog.NewTextHandler(testWriter{t}, nil)),
	})
	return &rotateFixture{store: store, core: c, pf: pf}
}

func TestSync_NoopWhenGatewayMatches(t *testing.T) {
	active := &gcp.ExitNode{Name: "v1", TailscaleIP: "100.64.0.99"}
	f := syncFixture(t, active, "100.64.0.99")
	if err := f.core.SyncPFSense(context.Background()); err != nil {
		t.Fatalf("SyncPFSense: %v", err)
	}
	want := []string{"GetGateway"}
	if !reflect.DeepEqual(f.pf.names(), want) {
		t.Errorf("calls = %v, want %v", f.pf.names(), want)
	}
}

func TestSync_PushesWhenGatewayDiffers(t *testing.T) {
	active := &gcp.ExitNode{Name: "v1", TailscaleIP: "100.64.0.99"}
	f := syncFixture(t, active, "100.64.0.1")
	if err := f.core.SyncPFSense(context.Background()); err != nil {
		t.Fatalf("SyncPFSense: %v", err)
	}
	want := []string{"GetGateway", "UpdateGatewayIP", "Apply"}
	if !reflect.DeepEqual(f.pf.names(), want) {
		t.Errorf("calls = %v, want %v", f.pf.names(), want)
	}
	if got := f.pf.Gateways["GW"]; got != "100.64.0.99" {
		t.Errorf("gateway = %s, want 100.64.0.99", got)
	}
}

func TestSync_NoActive(t *testing.T) {
	f := syncFixture(t, nil, "100.64.0.1")
	if err := f.core.SyncPFSense(context.Background()); !errors.Is(err, ErrNoActiveNode) {
		t.Errorf("err = %v, want ErrNoActiveNode", err)
	}
}
