package core

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/iker/exit-node/internal/config"
	"github.com/iker/exit-node/internal/gcp"
	"github.com/iker/exit-node/internal/state"
)

func costFixture(t *testing.T, active *gcp.ExitNode) *Core {
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
	return New(Deps{
		Config: &config.Config{}, Provider: newMockProvider(), TS: newMockTS(),
		PF: newMockPF(), Probe: &mockProbe{},
		Store: store, Logger: slog.New(slog.NewTextHandler(testWriter{t}, nil)),
	})
}

func TestEstimateCost_24h(t *testing.T) {
	active := &gcp.ExitNode{
		Name:        "v1",
		MachineType: "e2-micro",
		CreatedAt:   time.Now().Add(-24 * time.Hour),
	}
	c := costFixture(t, active)
	got, err := c.EstimateCost(context.Background(), CostOpts{Period: 24 * time.Hour})
	if err != nil {
		t.Fatalf("EstimateCost: %v", err)
	}
	// e2-micro static rate is $0.008/hr (per the table in cost.go).
	// 24h × $0.008 = $0.192.
	wantMin, wantMax := 0.18, 0.21
	if got.USD < wantMin || got.USD > wantMax {
		t.Errorf("USD = %.4f, want between %.2f and %.2f", got.USD, wantMin, wantMax)
	}
	if got.MachineType != "e2-micro" {
		t.Errorf("MachineType = %q", got.MachineType)
	}
}

func TestEstimateCost_UnknownMachine(t *testing.T) {
	active := &gcp.ExitNode{Name: "v1", MachineType: "fictional-1", CreatedAt: time.Now().Add(-1 * time.Hour)}
	c := costFixture(t, active)
	got, err := c.EstimateCost(context.Background(), CostOpts{Period: time.Hour})
	if err != nil {
		t.Fatalf("EstimateCost: %v", err)
	}
	// Unknown machine: USD stays 0, Note explains.
	if got.USD != 0 {
		t.Errorf("USD = %.4f, want 0 for unknown machine", got.USD)
	}
	if got.Note == "" {
		t.Errorf("Note should explain unknown machine type")
	}
}

func TestEstimateCost_NoActive(t *testing.T) {
	c := costFixture(t, nil)
	_, err := c.EstimateCost(context.Background(), CostOpts{Period: time.Hour})
	if err == nil {
		t.Errorf("expected ErrNoActiveNode-like error")
	}
}
