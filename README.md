# exit-node

[![Go Version](https://img.shields.io/badge/go-1.25-00ADD8?logo=go)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Status: Pre-alpha](https://img.shields.io/badge/status-pre--alpha-orange)](#status)

On-demand Tailscale exit nodes that rotate across cloud regions and keep your
pfSense gateway in sync. Driven from a CLI for humans and an MCP server for
AI agents.

> **Status**: pre-alpha (v0.1). All library packages, both binaries
> (`exitnode`, `exitnode-mcp`), CI, and release packaging are in
> place. The first tagged release will produce binaries for
> linux/darwin × amd64/arm64.

## Why

VPN exit nodes are useful exactly because they're not always-on:

- **Geo-bound testing.** Hop into `us-east1`, `europe-west1`, `asia-northeast1`
  on demand and tear it down when done.
- **Egress-IP rotation.** A new node every cycle, so your egress address moves
  without manual cloud-console clicking.
- **Tunnel discipline.** When you're done, the route disappears and your home
  gateway returns to whatever it was before — no leftover state.

Running a fleet of always-on exit nodes is wasteful and noisy. `exit-node`
provisions one when you ask, hands it to Tailscale, swaps your pfSense default
route to it, verifies egress, and destroys the old one cleanly.

## How it works

The rotate flow is a small state machine that runs end-to-end on `exitnode up`:

```
1. mint     →  ephemeral Tailscale auth-key (single-use, ~5min TTL)
2. provision →  GCP VM with metadata-baked auth-key + tags + hostname
3. wait     →  poll Tailscale until the device shows up connected
4. authorize →  set tags + approve subnet routes for the new device
5. probe    →  verify egress works through the candidate node
6. cut over →  pfSense gateway monitor IP swapped to the new node
7. verify   →  re-probe through the live route; restore prior node on failure
8. cleanup  →  destroy the previous VM, delete its Tailscale device
```

Every step is idempotent and observable. If verification fails post-cutover,
the prior gateway is restored before the candidate is destroyed.

## Architecture

Four narrow client interfaces, one orchestrator:

| Package | Responsibility |
| --- | --- |
| `internal/core` | Rotate state machine + `up` / `down` / `status` / `health` / `cost` / `list` / `sync` actions |
| `internal/config` | Typed configuration loader (project, region, tags, pfSense, gateway names) |
| `internal/state` | On-disk state file — what's currently running, last rotation, prior nodes |
| `internal/gcp` | Compute Engine adapter — `Provision` / `Start` / `Stop` / `Destroy` / `List` / `Get` |
| `internal/tailscale` | v2 API adapter — mint keys, wait for device, authorize, set tags, delete |
| `internal/pfsense` | pfSense REST adapter — read & swap gateway-monitor IP, apply |
| `internal/verify` | Egress probes — `EgressVia` (through a candidate) and `EgressDirect` (post-cutover) |

Each adapter is interface-first; tests run against fakes. The `gcp` package
also has a `//go:build integration` smoke test that exercises real GCP
(skipped unless `EXITNODE_INTEGRATION=1` and `EXITNODE_TEST_PROJECT` are set).

## Repository layout

```
exit-node/
├── internal/
│   ├── config/        # config loader
│   ├── state/         # on-disk state
│   ├── core/          # rotate orchestrator + action commands
│   ├── gcp/           # GCP Compute Engine adapter
│   ├── tailscale/     # Tailscale v2 API adapter
│   ├── pfsense/       # pfSense REST adapter
│   └── verify/        # egress probes
├── scripts/
│   └── install.sh     # VM first-boot bootstrap (fetched via startup-script-url)
├── Makefile
├── go.mod
└── README.md
```

## Prerequisites

- **GCP project** with the Compute Engine API enabled and a
  service account with `compute.instances.{create,start,stop,delete}`,
  `compute.zones.list`, and `compute.images.useReadOnly` on
  `projects/debian-cloud`. Either authenticate via Application
  Default Credentials (`gcloud auth application-default login`) or
  export the SA JSON in `GCP_CREDENTIALS_JSON`.
- **Tailscale tailnet** with an OAuth client (scopes:
  `auth_keys` write, `devices:core` write) and ACL `tagOwners`
  entries for whatever tag(s) you list in `tailscale.tags`. The
  tailnet must also have `autoApprovers.exitNode` set to your
  exit-node tag if you want approval to happen without manual
  admin intervention.
- **pfSense** with the
  [community pfsense-api plugin](https://github.com/jaredhendrickson13/pfsense-api)
  installed and an API key minted. The plugin must have the
  `Routing` permission group enabled for the key.
- **`tailscale` CLI** installed on the host that runs `exitnode`
  (used by the pre-cutover egress probe). If unavailable, the
  probe is skipped with a warning — see Troubleshooting.

## Install

Install both binaries via `go install`:

```bash
go install github.com/iker/exit-node/cmd/exitnode@latest
go install github.com/iker/exit-node/cmd/exitnode-mcp@latest
```

Or download a release archive from
[Releases](https://github.com/ceballosiker/exit-node/releases) and drop
`exitnode` + `exitnode-mcp` into your `$PATH`.

## Configure

Copy [`examples/config.toml`](examples/config.toml) to
`~/.config/exitnode/config.toml` and edit the values for your
environment. Every field is annotated in the example.

Export the secret env-vars referenced by your config:

```bash
export TAILSCALE_OAUTH_CLIENT_ID=...
export TAILSCALE_OAUTH_CLIENT_SECRET=...
export PFSENSE_API_KEY=...
# Optional — use a service-account JSON instead of ADC:
export GCP_CREDENTIALS_JSON="$(cat path/to/sa.json)"
```

Run `exitnode --help` to see the full command list. The most common flow:

```bash
exitnode up                    # provision (idempotent)
exitnode status                # see what's active
exitnode health                # probe egress
exitnode rotate --region us-east1   # cut over to a new region
exitnode down --destroy        # tear it all down
```

## Development

Requirements: Go 1.25+.

```bash
# Unit tests (race-enabled, fast)
make test

# Full suite incl. the build-tagged GCP integration test
#   (still skips unless EXITNODE_INTEGRATION=1 and EXITNODE_TEST_PROJECT are set)
make test-integration

# Vet
make vet

# Lint (requires golangci-lint)
make lint
```

The integration test against real GCP additionally honours:

| Env var | Default | Purpose |
| --- | --- | --- |
| `EXITNODE_INTEGRATION` | _unset_ | Must be `1` to run real-GCP tests |
| `EXITNODE_TEST_PROJECT` | _unset_ | GCP project ID to run against |
| `EXITNODE_TEST_REGION` | `us-central1` | Region for the smoke test |
| `GCP_CREDENTIALS_JSON` | _unset_ | Service-account JSON; otherwise ADC is used |

## Using exitnode-mcp

`exitnode-mcp` is a stdio MCP server that exposes the same operations to
AI agents (Claude Desktop, OpenClaw, any MCP host). Copy
[`examples/mcp.json`](examples/mcp.json) into your client's MCP
configuration. The binary must be on `$PATH`.

Available tools: `provision_exit_node`, `start_exit_node`,
`stop_exit_node`, `destroy_exit_node`, `rotate_exit_node`,
`list_exit_nodes`, `get_status`, `verify_connectivity`,
`sync_pfsense_gateway`, `estimate_cost`. `destroy_exit_node` requires
an explicit `name` argument — there is no "destroy active" shortcut
at the MCP layer, deliberately.

## Troubleshooting

**`tailscale OAuth env vars ... are unset`**
The env-var names declared in `[tailscale].oauth_client_id_env` and
`oauth_client_secret_env` are not set in the calling shell. Mint a new
OAuth client at `https://login.tailscale.com/admin/settings/oauth` and
export the two values. Check by running
`echo "$TAILSCALE_OAUTH_CLIENT_ID"` (or whatever name your config uses).

**pfSense revert failed — both nodes alive**
This is the loud-error case from the rotate state machine: after a
`pfSense Apply` failure, the orchestrator tried to revert to the old
gateway IP and that revert also failed. Both VMs remain running so your
home network keeps its VPN. Recover by manually setting the pfSense
gateway monitor IP to a known-good Tailscale IP from
`exitnode list`, then `exitnode down --destroy` to clean up. File an
issue with the orchestrator log so we can harden this path.

**`exitnode up` says "no `tailscale` CLI on host"**
The pre-cutover egress probe shells out to `tailscale`. Install the
client (`curl -fsSL https://tailscale.com/install.sh | sh` on Linux,
`brew install tailscale` on macOS) and re-run. The probe is skipped
silently if the CLI is absent — it does not block provisioning, but
you lose verification.

**"another exitnode process is running (state.json locked)"**
The state file's POSIX flock is held by another `exitnode` or
`exitnode-mcp` invocation. Most often this means a previous run
hung. Confirm with `lsof ~/.config/exitnode/state.json.lock`; if
nothing holds it, the lock file is stale and safe to delete with
`rm ~/.config/exitnode/state.json.lock`.

## Roadmap

Work is staged in three sequential plans:

- **Plan 1 — Foundation & Core.** Module layout, `internal/config`,
  `internal/state`, interface definitions for all four clients, and the
  `internal/core` rotate orchestrator with table-driven mocked tests.
  ✅ Complete.
- **Plan 2 — Real client implementations.** `internal/gcp`,
  `internal/tailscale`, `internal/pfsense`, `internal/verify`, and
  `scripts/install.sh`. ✅ Complete.
- **Plan 3 — Binaries & distribution.** `cmd/exitnode`, `cmd/exitnode-mcp`,
  examples, CI workflows, and goreleaser packaging. ✅ Complete.

## License

[MIT](LICENSE) © 2026 Iker.
