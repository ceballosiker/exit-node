package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/iker/exit-node/internal/core"
)

// --- Typed argument structs ---

type provisionArgs struct {
	Region      string `json:"region,omitempty"       jsonschema:"GCP region; omit to use the config default"`
	MachineType string `json:"machine_type,omitempty" jsonschema:"GCP machine type; omit to use the config default"`
}

type nameArgs struct {
	Name string `json:"name" jsonschema:"Exit-node name (matches the GCP VM name)"`
}

type nameOptionalArgs struct {
	Name string `json:"name,omitempty" jsonschema:"Exit-node name; omit to use the currently active node"`
}

type rotateArgs struct {
	Region string `json:"region,omitempty" jsonschema:"GCP region for the replacement node; omit to use the config default"`
}

type costArgs struct {
	Period string `json:"period,omitempty" jsonschema:"Look-back window: 24h, 7d, month; omit for since-creation"`
}

// --- registerTools wires every tool ---

func registerTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "provision_exit_node",
		Description: "Provision (or return the existing) active exit node. Idempotent.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a provisionArgs) (*mcp.CallToolResult, any, error) {
		c, cleanup, err := buildCore(ctx)
		if err != nil {
			return nil, nil, err
		}
		defer cleanup()
		node, err := c.Up(ctx, core.UpOpts{Region: a.Region, MachineType: a.MachineType})
		return jsonResult(node, err)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "start_exit_node",
		Description: "Start a stopped exit node by name (defaults to the currently active node).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a nameOptionalArgs) (*mcp.CallToolResult, any, error) {
		c, cleanup, err := buildCore(ctx)
		if err != nil {
			return nil, nil, err
		}
		defer cleanup()
		name, err := resolveName(c, a.Name)
		if err != nil {
			return nil, nil, err
		}
		err = c.Start(ctx, name)
		return jsonResult(map[string]string{"started": name}, err)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "stop_exit_node",
		Description: "Stop a running exit node by name (defaults to the currently active node).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a nameOptionalArgs) (*mcp.CallToolResult, any, error) {
		c, cleanup, err := buildCore(ctx)
		if err != nil {
			return nil, nil, err
		}
		defer cleanup()
		name, err := resolveName(c, a.Name)
		if err != nil {
			return nil, nil, err
		}
		err = c.Stop(ctx, name)
		return jsonResult(map[string]string{"stopped": name}, err)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "destroy_exit_node",
		Description: "Destroy an exit-node VM + its Tailscale device. Name is REQUIRED — there is no implicit 'destroy active' here.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a nameArgs) (*mcp.CallToolResult, any, error) {
		if a.Name == "" {
			return nil, nil, fmt.Errorf("name is required for destroy_exit_node")
		}
		c, cleanup, err := buildCore(ctx)
		if err != nil {
			return nil, nil, err
		}
		defer cleanup()
		err = c.Destroy(ctx, a.Name)
		return jsonResult(map[string]string{"destroyed": a.Name}, err)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "rotate_exit_node",
		Description: "Provision a new exit node, cut pfSense over, destroy the old one.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a rotateArgs) (*mcp.CallToolResult, any, error) {
		c, cleanup, err := buildCore(ctx)
		if err != nil {
			return nil, nil, err
		}
		defer cleanup()
		res, err := c.Rotate(ctx, core.RotateOpts{Region: a.Region})
		return jsonResult(res, err)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_exit_nodes",
		Description: "List all exitnode-managed VMs in the GCP project.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		c, cleanup, err := buildCore(ctx)
		if err != nil {
			return nil, nil, err
		}
		defer cleanup()
		nodes, err := c.List(ctx)
		return jsonResult(nodes, err)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_status",
		Description: "Get cached + live status of the active exit node, plus a drift flag.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		c, cleanup, err := buildCore(ctx)
		if err != nil {
			return nil, nil, err
		}
		defer cleanup()
		st, err := c.Status(ctx)
		return jsonResult(st, err)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "verify_connectivity",
		Description: "Probe egress through the active exit node and return ok / egress_ip / expected_ip.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		c, cleanup, err := buildCore(ctx)
		if err != nil {
			return nil, nil, err
		}
		defer cleanup()
		h, err := c.Health(ctx)
		return jsonResult(h, err)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "sync_pfsense_gateway",
		Description: "Push the active node's Tailscale IP to the pfSense gateway-monitor IP + apply.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		c, cleanup, err := buildCore(ctx)
		if err != nil {
			return nil, nil, err
		}
		defer cleanup()
		err = c.SyncPFSense(ctx)
		return jsonResult(map[string]string{"status": "ok"}, err)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "estimate_cost",
		Description: "Estimate spend for the active exit node over a look-back window.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a costArgs) (*mcp.CallToolResult, any, error) {
		c, cleanup, err := buildCore(ctx)
		if err != nil {
			return nil, nil, err
		}
		defer cleanup()
		var d time.Duration
		if a.Period != "" {
			p, perr := parsePeriodMCP(a.Period)
			if perr != nil {
				return nil, nil, perr
			}
			d = p
		}
		r, err := c.EstimateCost(ctx, core.CostOpts{Period: d})
		return jsonResult(r, err)
	})
}

// resolveName falls back to the cached active node when the caller
// passed an empty name (used by start/stop tools).
func resolveName(c *core.Core, name string) (string, error) {
	if name != "" {
		return name, nil
	}
	active, err := c.GetActive()
	if err != nil {
		return "", fmt.Errorf("resolve active node: %w", err)
	}
	if active == nil {
		return "", fmt.Errorf("name omitted and no active exit node recorded")
	}
	return active.Name, nil
}

// jsonResult marshals v to JSON text content if err is nil; otherwise
// returns the error to the SDK which renders it as an isError result.
func jsonResult(v any, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return nil, nil, err
	}
	b, mErr := json.MarshalIndent(v, "", "  ")
	if mErr != nil {
		return nil, nil, fmt.Errorf("marshal result: %w", mErr)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(b)}},
	}, v, nil
}

// parsePeriodMCP parses a human look-back window string (24h, 7d, month)
// into a time.Duration. Same semantics as the CLI's parsePeriod helper.
func parsePeriodMCP(s string) (time.Duration, error) {
	if s == "month" {
		return 30 * 24 * time.Hour, nil
	}
	if len(s) > 1 && s[len(s)-1] == 'd' {
		var days int
		if _, err := fmt.Sscanf(s, "%dd", &days); err == nil {
			return time.Duration(days) * 24 * time.Hour, nil
		}
	}
	return time.ParseDuration(s)
}
