// Package state implements the on-disk cache for exit-node state. The file
// is treated as a best-effort cache; GCP labels + Tailscale tags are the
// source of truth. A POSIX file lock prevents concurrent rotates.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
	"github.com/iker/exit-node/internal/gcp"
)

// ErrLocked is returned when another process holds the lock.
var ErrLocked = errors.New("state file already locked by another process")

// Store is a flock-protected JSON cache for the current active exit node.
type Store struct {
	path string
	lock *flock.Flock
}

// Open creates the parent directory if needed and acquires a non-blocking
// exclusive lock on the state file. The lock is released by Close.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir state parent: %w", err)
	}
	l := flock.New(path + ".lock")
	got, err := l.TryLock()
	if err != nil {
		return nil, fmt.Errorf("acquire state lock: %w", err)
	}
	if !got {
		return nil, ErrLocked
	}
	return &Store{path: path, lock: l}, nil
}

// Close releases the file lock. Idempotent.
func (s *Store) Close() error {
	if s.lock == nil {
		return nil
	}
	err := s.lock.Unlock()
	s.lock = nil
	return err
}

type onDisk struct {
	Active *gcp.ExitNode `json:"active,omitempty"`
}

func (s *Store) read() (*onDisk, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return &onDisk{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	if len(b) == 0 {
		return &onDisk{}, nil
	}
	var d onDisk
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	return &d, nil
}

func (s *Store) write(d *onDisk) error {
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("write tmp state: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("rename state: %w", err)
	}
	return nil
}

// GetActive returns the cached active exit node, or nil if none recorded.
func (s *Store) GetActive() (*gcp.ExitNode, error) {
	d, err := s.read()
	if err != nil {
		return nil, err
	}
	return d.Active, nil
}

// SetActive records the given node as the active one.
func (s *Store) SetActive(n *gcp.ExitNode) error {
	d, err := s.read()
	if err != nil {
		return err
	}
	d.Active = n
	return s.write(d)
}

// ClearActive removes the active-node record.
func (s *Store) ClearActive() error {
	d, err := s.read()
	if err != nil {
		return err
	}
	d.Active = nil
	return s.write(d)
}
