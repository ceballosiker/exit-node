package state

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/iker/exit-node/internal/gcp"
)

func TestSaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	active := &gcp.ExitNode{
		Name:        "vpn-us-west1-a-xyz1",
		Region:      "us-west1",
		Zone:        "us-west1-a",
		MachineType: "e2-micro",
		PublicIP:    "35.1.1.1",
		TailscaleIP: "100.64.0.1",
		DeviceID:    "dev1",
		State:       gcp.StateRunning,
		CreatedAt:   time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC),
	}
	if err := s.SetActive(active); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close first: %v", err)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()

	got, err := s2.GetActive()
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	if got == nil {
		t.Fatalf("got nil active node, expected one")
	}
	if got.Name != active.Name {
		t.Errorf("Name = %q, want %q", got.Name, active.Name)
	}
	if got.State != gcp.StateRunning {
		t.Errorf("State = %v, want StateRunning", got.State)
	}
	if !got.CreatedAt.Equal(active.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, active.CreatedAt)
	}
}

func TestGetActiveOnEmptyState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	got, err := s.GetActive()
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for empty state, got %+v", got)
	}
}

func TestClearActive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if err := s.SetActive(&gcp.ExitNode{Name: "x"}); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	if err := s.ClearActive(); err != nil {
		t.Fatalf("ClearActive: %v", err)
	}
	got, err := s.GetActive()
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil after Clear, got %+v", got)
	}
}

func TestFlockContention(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	first, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	defer first.Close()

	_, err = Open(path)
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("second Open: got err=%v, want ErrLocked", err)
	}

	// After releasing the first, a second Open succeeds.
	if err := first.Close(); err != nil {
		t.Fatalf("close first: %v", err)
	}
	second, err := Open(path)
	if err != nil {
		t.Fatalf("Open after release: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("close second: %v", err)
	}
}
