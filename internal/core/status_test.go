package core

import (
	"context"
	"log/slog"
	"testing"

	"github.com/iker/exit-node/internal/config"
	"github.com/iker/exit-node/internal/gcp"
	"github.com/iker/exit-node/internal/state"
)

func TestStatus_ActiveExists(t *testing.T) {
	statePath := t.TempDir() + "/state.json"
	store, _ := state.Open(statePath)
	defer store.Close()
	active := &gcp.ExitNode{Name: "v1", State: gcp.StateStopped}
	_ = store.SetActive(active)

	prov := newMockProvider()
	// Provider now reports Running — drift from state cache.
	prov.GetResult["v1"] = &gcp.ExitNode{Name: "v1", State: gcp.StateRunning}
	c := New(Deps{
		Config: &config.Config{}, Provider: prov, TS: newMockTS(),
		PF: newMockPF(), Probe: &mockProbe{},
		Store: store, Logger: slog.New(slog.NewTextHandler(testWriter{t}, nil)),
	})
	got, err := c.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if got.Cached == nil || got.Cached.Name != "v1" {
		t.Errorf("Cached = %v", got.Cached)
	}
	if got.Live == nil || got.Live.State != gcp.StateRunning {
		t.Errorf("Live = %v", got.Live)
	}
	if !got.Drift() {
		t.Errorf("expected Drift=true")
	}
}

func TestStatus_NoActive(t *testing.T) {
	statePath := t.TempDir() + "/state.json"
	store, _ := state.Open(statePath)
	defer store.Close()
	c := New(Deps{
		Config: &config.Config{}, Provider: newMockProvider(), TS: newMockTS(),
		PF: newMockPF(), Probe: &mockProbe{},
		Store: store, Logger: slog.New(slog.NewTextHandler(testWriter{t}, nil)),
	})
	got, err := c.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if got.Cached != nil || got.Live != nil {
		t.Errorf("expected nil/nil, got %+v", got)
	}
}
