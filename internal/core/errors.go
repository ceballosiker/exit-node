package core

import "fmt"

// CriticalError signals an unrecoverable rotate failure where the pfSense
// revert itself failed. Callers MUST surface this loudly: both the old and
// new nodes are likely alive and pfSense's gateway state is ambiguous.
type CriticalError struct {
	Message     string
	PrimaryErr  error
	RevertErr   error
	NewNodeName string
	NewDeviceID string
}

func (e *CriticalError) Error() string {
	return fmt.Sprintf("CRITICAL: %s: primary_err=%v; revert_err=%v; new_node=%s; new_device=%s",
		e.Message, e.PrimaryErr, e.RevertErr, e.NewNodeName, e.NewDeviceID)
}

func (e *CriticalError) Unwrap() error { return e.PrimaryErr }
