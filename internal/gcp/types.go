// Package gcp defines the GCP-backed exit-node types and the Provider
// interface. The concrete implementation lives in this package but is added
// in Plan 2; Plan 1 only sets the contract that internal/core consumes.
package gcp

import (
	"fmt"
	"strings"
	"time"
)

// State is the lifecycle state of an exit-node VM.
type State int

const (
	StateUnknown State = iota
	StatePending
	StateRunning
	StateStopped
	StateTerminated
)

func (s State) String() string {
	switch s {
	case StatePending:
		return "pending"
	case StateRunning:
		return "running"
	case StateStopped:
		return "stopped"
	case StateTerminated:
		return "terminated"
	default:
		return "unknown"
	}
}

// ParseState parses a state string (case-insensitive). Unknown inputs return
// StateUnknown with a non-nil error.
func ParseState(s string) (State, error) {
	switch strings.ToLower(s) {
	case "pending":
		return StatePending, nil
	case "running":
		return StateRunning, nil
	case "stopped":
		return StateStopped, nil
	case "terminated":
		return StateTerminated, nil
	case "unknown":
		return StateUnknown, nil
	default:
		return StateUnknown, fmt.Errorf("unknown state %q", s)
	}
}

// ExitNode is the canonical representation of a managed exit-node VM.
type ExitNode struct {
	Name        string
	Region      string
	Zone        string
	MachineType string
	PublicIP    string
	TailscaleIP string
	DeviceID    string
	State       State
	CreatedAt   time.Time
}

// ProvisionOpts is the input to Provider.Provision.
type ProvisionOpts struct {
	Name             string // generated: vpn-<region>-<zone>-<rand>
	Region           string
	Zone             string // empty → provider chooses a random zone in Region
	MachineType      string
	Hostname         string   // typically == Name
	TailscaleAuthKey string   // ephemeral, single-use, ~5m TTL
	Tags             []string // applied as both GCP labels and Tailscale tags
	InstallScriptURL string
	DiskSizeGB       int
	Network          string
}
