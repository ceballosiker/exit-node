# exit-node

[![Go Version](https://img.shields.io/badge/go-1.25-00ADD8?logo=go)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Status: Pre-alpha](https://img.shields.io/badge/status-pre--alpha-orange)](#status)

On-demand Tailscale exit nodes that rotate across cloud regions and keep your
pfSense gateway in sync. Driven from a CLI for humans and an MCP server for
AI agents.

> **Status**: pre-alpha. The library packages (provisioning, rotate
> orchestrator, client adapters, probes) are in place. The `exitnode` and
> `exitnode-mcp` binaries are not yet built — see [Roadmap](#roadmap).

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
  examples, CI workflows, and goreleaser packaging. ⏳ In progress.

## License

[MIT](LICENSE) © 2026 Iker.
