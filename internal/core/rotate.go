package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/iker/exit-node/internal/gcp"
	"github.com/iker/exit-node/internal/tailscale"
)

// RotateOpts controls a single rotate invocation.
type RotateOpts struct {
	Region      string // empty → cfg.GCP.DefaultRegion
	MachineType string // empty → cfg.GCP.DefaultMachineType
}

// RotateResult captures both the destroyed and provisioned nodes.
type RotateResult struct {
	Old *gcp.ExitNode
	New *gcp.ExitNode
}

// Rotate provisions a new exit node, verifies it, optionally cuts pfSense
// over, then tears down the old one. See spec §6 for the rollback rules.
func (c *Core) Rotate(ctx context.Context, opts RotateOpts) (*RotateResult, error) {
	region := opts.Region
	if region == "" {
		region = c.cfg.GCP.DefaultRegion
	}
	machine := opts.MachineType
	if machine == "" {
		machine = c.cfg.GCP.DefaultMachineType
	}

	// 1. Snapshot current state.
	old, err := c.store.GetActive()
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}

	// 2. Mint ephemeral auth key.
	authKey, err := c.ts.MintEphemeralAuthKey(ctx, c.cfg.Tailscale.Tags)
	if err != nil {
		return nil, fmt.Errorf("mint ephemeral auth key: %w", err)
	}

	// 3. Provision new VM.
	name := genName(region)
	newNode, err := c.provider.Provision(ctx, gcp.ProvisionOpts{
		Name:             name,
		Region:           region,
		Zone:             c.cfg.GCP.DefaultZone,
		MachineType:      machine,
		Hostname:         name,
		TailscaleAuthKey: authKey,
		Tags:             c.cfg.Tailscale.Tags,
		InstallScriptURL: c.cfg.Behavior.InstallScriptURL,
		DiskSizeGB:       c.cfg.GCP.DiskSizeGB,
		Network:          c.cfg.GCP.Network,
	})
	if err != nil {
		return nil, fmt.Errorf("provision new node: %w", err)
	}

	// From here on, newNode exists in GCP. Track cleanup.
	var newDevice *tailscale.Device
	cleanup := func(reason error) {
		c.log.Warn("rotate failed, tearing down new node",
			"reason", reason, "name", newNode.Name)
		cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if newDevice != nil {
			if dErr := c.ts.DeleteDevice(cctx, newDevice.ID); dErr != nil {
				c.log.Error("cleanup DeleteDevice failed", "err", dErr)
			}
		}
		if dErr := c.provider.Destroy(cctx, newNode.Name); dErr != nil {
			c.log.Error("cleanup Destroy failed", "err", dErr)
		}
	}

	// 4. Wait for Tailscale registration.
	newDevice, err = c.ts.WaitForDevice(ctx, newNode.Name, c.cfg.Behavior.RegistrationTimeout)
	if err != nil {
		cleanup(fmt.Errorf("device never registered: %w", err))
		return nil, fmt.Errorf("wait for device: %w", err)
	}

	// 5. Authorize as exit node.
	if err := c.ts.AuthorizeExitNode(ctx, newDevice.ID); err != nil {
		cleanup(fmt.Errorf("authorize as exit node: %w", err))
		return nil, fmt.Errorf("authorize exit node: %w", err)
	}

	// 6. Set tags.
	if err := c.ts.SetTags(ctx, newDevice.ID, c.cfg.Tailscale.Tags); err != nil {
		cleanup(fmt.Errorf("set tags: %w", err))
		return nil, fmt.Errorf("set tags: %w", err)
	}

	newNode.TailscaleIP = newDevice.TailscaleIP
	newNode.DeviceID = newDevice.ID

	// 7. Pre-cutover probe.
	if c.cfg.Behavior.VerifyPreCutover {
		egress, err := c.probe.EgressVia(ctx, newDevice.TailscaleIP)
		if err != nil {
			cleanup(fmt.Errorf("probe failed: %w", err))
			return nil, fmt.Errorf("pre-cutover probe: %w", err)
		}
		if egress != newNode.PublicIP {
			cleanup(fmt.Errorf("egress mismatch: probe=%s vm=%s", egress, newNode.PublicIP))
			return nil, fmt.Errorf("egress IP mismatch: got %s, want %s", egress, newNode.PublicIP)
		}
	}

	// (pfSense branch added in Task 10)

	// Tear down old node (best-effort).
	if old != nil {
		if err := c.provider.Destroy(ctx, old.Name); err != nil {
			c.log.Error("teardown old VM failed (best-effort)",
				"err", err, "name", old.Name)
		}
		if old.DeviceID != "" {
			if err := c.ts.DeleteDevice(ctx, old.DeviceID); err != nil {
				c.log.Error("teardown old device failed (best-effort)",
					"err", err, "device_id", old.DeviceID)
			}
		}
	}

	// Update state cache.
	if err := c.store.SetActive(newNode); err != nil {
		c.log.Warn("failed to update state cache; will reconcile next run",
			"err", err)
	}

	return &RotateResult{Old: old, New: newNode}, nil
}

// genName returns a name in the convention vpn-<region>-<6-hex>.
func genName(region string) string {
	var b [3]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("vpn-%s-%s", region, hex.EncodeToString(b[:]))
}
