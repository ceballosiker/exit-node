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

func healthFixture(t *testing.T, active *gcp.ExitNode, probeIP string, probeErr error) *Core {
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
	prov := newMockProvider()
	if active != nil {
		prov.GetResult[active.Name] = active
	}
	probe := &mockProbe{EgressViaResult: probeIP, EgressViaErr: probeErr}
	return New(Deps{
		Config:   &config.Config{Behavior: config.BehaviorConfig{ProbeURL: "https://x/ip"}},
		Provider: prov, TS: newMockTS(), PF: newMockPF(), Probe: probe,
		Store: store, Logger: slog.New(slog.NewTextHandler(testWriter{t}, nil)),
	})
}

func TestHealth_OK(t *testing.T) {
	active := &gcp.ExitNode{Name: "v1", PublicIP: "1.2.3.4", TailscaleIP: "100.64.0.1"}
	c := healthFixture(t, active, "1.2.3.4", nil)
	got, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if !got.OK {
		t.Errorf("OK = false, want true; got=%+v", got)
	}
	if got.EgressIP != "1.2.3.4" {
		t.Errorf("EgressIP = %q", got.EgressIP)
	}
}

func TestHealth_Mismatch(t *testing.T) {
	active := &gcp.ExitNode{Name: "v1", PublicIP: "1.2.3.4", TailscaleIP: "100.64.0.1"}
	c := healthFixture(t, active, "5.6.7.8", nil)
	got, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if got.OK {
		t.Errorf("OK = true, want false")
	}
	if got.EgressIP != "5.6.7.8" || got.ExpectedIP != "1.2.3.4" {
		t.Errorf("got %+v", got)
	}
}

func TestHealth_NoActive(t *testing.T) {
	c := healthFixture(t, nil, "", nil)
	_, err := c.Health(context.Background())
	if !errors.Is(err, ErrNoActiveNode) {
		t.Errorf("err = %v, want ErrNoActiveNode", err)
	}
}
