package core

import (
	"context"
	"log/slog"
	"testing"

	"github.com/iker/exit-node/internal/config"
	"github.com/iker/exit-node/internal/gcp"
	"github.com/iker/exit-node/internal/state"
)

func TestList(t *testing.T) {
	statePath := t.TempDir() + "/state.json"
	store, err := state.Open(statePath)
	if err != nil {
		t.Fatalf("state.Open: %v", err)
	}
	defer store.Close()
	prov := newMockProvider()
	prov.ListResult = []*gcp.ExitNode{
		{Name: "a", State: gcp.StateRunning},
		{Name: "b", State: gcp.StateStopped},
	}
	c := New(Deps{
		Config: &config.Config{}, Provider: prov, TS: newMockTS(),
		PF: newMockPF(), Probe: &mockProbe{},
		Store: store, Logger: slog.New(slog.NewTextHandler(testWriter{t}, nil)),
	})
	got, err := c.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 || got[0].Name != "a" || got[1].Name != "b" {
		t.Errorf("got %v", got)
	}
}
