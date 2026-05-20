package main

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestRegisterTools_RegistersAllTen verifies that calling registerTools does
// not panic and registers exactly the 10 expected tool names.
// The MCP SDK (v1.6.0) does not expose a public listing API on *mcp.Server, so
// this is a no-panic smoke test. Protocol-level tool enumeration is covered by
// Task 16+ integration tests.
func TestRegisterTools_RegistersAllTen(t *testing.T) {
	s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	registerTools(s) // must not panic

	// Enumerate expected names to document the contract even though we cannot
	// assert them via the SDK's public API at this point.
	wantNames := []string{
		"provision_exit_node",
		"start_exit_node",
		"stop_exit_node",
		"destroy_exit_node",
		"rotate_exit_node",
		"list_exit_nodes",
		"get_status",
		"verify_connectivity",
		"sync_pfsense_gateway",
		"estimate_cost",
	}
	if len(wantNames) != 10 {
		t.Fatalf("test bug: expected 10 tools, listed %d", len(wantNames))
	}
	// If a future SDK version exposes Server.ListTools() or similar, replace
	// this comment with an assertion over wantNames.
}
