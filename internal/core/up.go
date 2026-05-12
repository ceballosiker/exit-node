package core

import (
	"context"
	"fmt"

	"github.com/iker/exit-node/internal/gcp"
)

// UpOpts controls a single Up invocation.
type UpOpts struct {
	Region      string // empty → cfg.GCP.DefaultRegion
	MachineType string // empty → cfg.GCP.DefaultMachineType
}

// Up provisions an exit node if none is recorded as active. If one already
// exists, returns it unchanged (idempotent).
func (c *Core) Up(ctx context.Context, opts UpOpts) (*gcp.ExitNode, error) {
	if existing, err := c.store.GetActive(); err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	} else if existing != nil {
		return existing, nil
	}

	region := opts.Region
	if region == "" {
		region = c.cfg.GCP.DefaultRegion
	}
	machine := opts.MachineType
	if machine == "" {
		machine = c.cfg.GCP.DefaultMachineType
	}

	authKey, err := c.ts.MintEphemeralAuthKey(ctx, c.cfg.Tailscale.Tags)
	if err != nil {
		return nil, fmt.Errorf("mint ephemeral auth key: %w", err)
	}
	name := genName(region)
	node, err := c.provider.Provision(ctx, gcp.ProvisionOpts{
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
		return nil, fmt.Errorf("provision: %w", err)
	}
	dev, err := c.ts.WaitForDevice(ctx, node.Name, c.cfg.Behavior.RegistrationTimeout)
	if err != nil {
		// Best-effort destroy on registration failure to avoid orphans.
		_ = c.provider.Destroy(context.Background(), node.Name)
		return nil, fmt.Errorf("wait for device: %w", err)
	}
	if err := c.ts.AuthorizeExitNode(ctx, dev.ID); err != nil {
		_ = c.ts.DeleteDevice(context.Background(), dev.ID)
		_ = c.provider.Destroy(context.Background(), node.Name)
		return nil, fmt.Errorf("authorize: %w", err)
	}
	if err := c.ts.SetTags(ctx, dev.ID, c.cfg.Tailscale.Tags); err != nil {
		_ = c.ts.DeleteDevice(context.Background(), dev.ID)
		_ = c.provider.Destroy(context.Background(), node.Name)
		return nil, fmt.Errorf("set tags: %w", err)
	}
	node.TailscaleIP = dev.TailscaleIP
	node.DeviceID = dev.ID

	if c.cfg.Behavior.VerifyPreCutover {
		egress, err := c.probe.EgressVia(ctx, dev.TailscaleIP)
		if err != nil {
			_ = c.ts.DeleteDevice(context.Background(), dev.ID)
			_ = c.provider.Destroy(context.Background(), node.Name)
			return nil, fmt.Errorf("probe: %w", err)
		}
		if egress != node.PublicIP {
			_ = c.ts.DeleteDevice(context.Background(), dev.ID)
			_ = c.provider.Destroy(context.Background(), node.Name)
			return nil, fmt.Errorf("egress IP mismatch: got %s, want %s", egress, node.PublicIP)
		}
	}

	if err := c.store.SetActive(node); err != nil {
		c.log.Warn("failed to update state cache", "err", err)
	}
	return node, nil
}
