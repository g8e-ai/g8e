---
doc_id: gateway
title: Gateway Architecture
audience: maintainers and coding agents
status: current
last_updated: 2026-09-28
version: v2.2.3
owners:
  - internal/services/gateway/
  - internal/cli/cmd/gw/
  - protocol/proto/g8e/common/v1/
related:
  - network.md
  - operator.md
  - evals.md
  - public_spectator.md
  - sse.md
  - storage.md
when_to_read: Understanding the Governance Gateway architecture, operational modes, MCP endpoint implementation, governance layers, HTTP routing, and session management.
do_not_use_for:
  - Network topology and PKI hierarchy (see network.md)
  - Operator architecture and cross-gateway enrollment (see operator.md)
  - Storage and persistence details (see storage.md)
---

# Gateway Architecture

## Purpose

Documents the g8e Governance Gateway (g8eg) architecture: its operational modes, 5-layer governance enforcement, MCP endpoint, HTTP routing, session types, and agent integration. The Gateway is the protocol hub and policy decision point (PDP) that coordinates L1-L3 governance, manages PKI, runs pub/sub, and owns local audit evidence. This document maps architecture to implementation in [internal/services/gateway/](internal/services/gateway/) and [internal/cli/cmd/gw/](internal/cli/cmd/gw/).

## Quick index

- [Purpose](#purpose)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

## Invariants

| ID | Rule |
| --- | --- |
| INV-GW-01 | The Gateway runs in one of four postures: `doctrine` (L1 enforced, L2/L3 audited), `consensus` (L1/L2 enforced, L3 audited), `ratify` (L1/L3 enforced, L2 audited), `notary` (L1/L2/L3 strictly enforced). Posture is set once at `gw start` via `--posture` flag with no runtime change. |
| INV-GW-02 | The Gateway exposes two logical protocol surfaces with distinct authentication requirements. HTTP port 8080 carries plain-text bootstrap, PKI discovery, health, and CLI recovery flows; HTTPS port 8443 carries MCP, API, console, and pub/sub traffic with optional or required client certificates per route. |
| INV-GW-03 | Every transaction dispatched to `POST /api/v1/governance/envelopes` passes through all five governance layers (L1 Doctrine, L2 Consensus, L3 Notary, L4 Warden, L5 Actuator). The Gateway owns L1-L3; the Operator substrate (in-process or remote) owns L4-L5. |
| INV-GW-04 | The Gateway maintains a native MCP tool registry in [internal/services/mcp/native_tool_registry.go](internal/services/mcp/native_tool_registry.go) containing 31 tools across database, filesystem, network, process, system, cloud, and audit categories. All tools enforce fail-closed input validation per category (SQL, URL, path, protocol, hostname, cloud metadata). |
| INV-GW-05 | Session types are immutable bindings: `operator_session_id` (mTLS Operator certificate), `cli_session_id` (mTLS CLI certificate), `web_session_id` (WebAuthn passkey), `sub` (JWT user ID from external IdP). A single request carries exactly one session identifier. |
| INV-GW-06 | Launch profiles written by `gw start` to `.g8e/pids/operator-launch-profile.json` are versioned and mandatory for `gw restart`. Restart fails closed if the profile is missing, malformed, or unknown-version rather than falling back to defaults. |
| INV-GW-07 | The Gateway's HTTP router assigns each route to one of four auth modes: public (health, PKI, bootstrap), mTLS-only (governance, operator, audit, pub/sub), web-session-only (user profile, approvals, credential management), dual (SSE accepts mTLS or web session). Auth mode mismatches are routing errors. |

## Owned surfaces

| Surface | Path | Verify |
| --- | --- | --- |
| Gateway binary (Linux) | `bin/g8e` | SHA256 in release notes; distributed via HTTP from `/download/bin/g8e/<version>/` |
| Gateway startup command | `internal/cli/cmd/gw/gateway.go` | `gw start`, `--posture` flag with four values, launch profile write to `.g8e/pids/` |
| HTTP/HTTPS port constants | `internal/constants/network.go` | `GatewayHTTPPort = "8080"`, `GatewayHTTPSPort = "8443"` |
| Governance layers | `internal/services/governance/` | Five files: `l1_doctrine.go`, `l3_notary.go`, `l4_warden.go`, `l5_actuator.go`; L2 Consensus delegated to enrolled service |
| MCP native tools | `internal/services/mcp/native_tool_registry.go` | All 31 tools registered at startup; input validation per category enforced |
| HTTP router and auth | `internal/services/gateway/router.go` | Routes classified as public, mTLS-only, web-session-only, dual |
| Session types | `protocol/proto/g8e/common/v1/common.proto` | `operator_session_id`, `web_session_id`, `cli_session_id` fields in GovernanceEnvelope |
| Interactive wizard | `internal/cli/cmd/gw/gateway_setup.go` | `gw setup`, `gw start -i` launch TUI; output merged with resolved CLI flags |

## Procedures

### Operating Modes

**Gateway Mode (`gw start`)**

Starts the Governance Gateway with a specified posture. The binary runs as the platform's central backbone: protocol hub, policy decision point, SQLite persistence owner, pub/sub broker, PKI authority, and audit coordinator. The Gateway constructs or admits governed transactions, owns L1-L3 policy decisions, and runs an in-process Operator substrate for L4-L5 enforcement against its own runtime.

Commands:
- `g8e gw start [--posture doctrine|consensus|ratify|notary] [--http-port 8080] [--https-port 8443]` — Starts Gateway with optional port overrides.
- `g8e gw status` — Reports health and Docker Compose stack status (if running in Compose).
- `g8e gw restart` — Reads persisted launch profile from `.g8e/pids/operator-launch-profile.json` and re-runs network identity detection before restart. Fails closed if profile is missing or malformed.
- `g8e gw stop` — Gracefully shuts down the Gateway.

**Operator Mode (`operator start`)**

The same binary run in Standard Mode becomes a Governed Operator (g8eo), the sovereign execution boundary for its runtime. It opens an outbound-only mTLS connection to the Gateway, receives commands on its bound session, re-verifies the envelope and required L1-L3 proofs, and handles L4-L5 locally. The Gateway's unified MCP endpoint is the client-facing ingress; remote Operators do not receive inbound connections.

### Configuration and Enrollment

**Interactive Setup Wizard**

- `g8e gw setup` — Launches an interactive TUI without starting the Gateway, producing a resolved configuration for inspection.
- `g8e gw start -i` (or `--interactive`) — Launches the same wizard, then starts the Gateway with the configuration.

The wizard guides users through: Network & Identity, Security & Governance Posture, Agent Tooling & Routing, and Review & Confirm. Output is merged into resolved CLI flags, preserving non-wizard flags (ports, log level, rate limits). Cancellation returns without starting.

**Frontend Connection**

- `g8e gw connect <frontend-origin>` — One-command workflow for connecting a browser-hosted frontend on the same computer. Validates and normalizes origin, derives exact-host WebAuthn RP ID, starts or restarts Gateway with matching CORS and passkey settings when needed (with explicit consent), installs or refreshes local gateway root trust, verifies HTTPS and CORS, and prints a frontend handoff prompt.

### Governance Postures

All postures enforce L1 Doctrine (forbidden-pattern matching and MITRE threat detection). Posture differences control L2 Consensus and L3 Notary requirements:

| Posture | L1 | L2 Consensus | L3 Notary | Typical Use |
| --- | --- | --- | --- | --- |
| `doctrine` | Enforced | Audited | Audited | Development, testing, local deployments |
| `consensus` | Enforced | Enforced | Audited | Team coordination requiring consensus votes |
| `ratify` | Enforced | Audited | Enforced | Human authorization required; consensus optional |
| `notary` | Enforced | Enforced | Enforced | Strict multi-signature governance with human approval |

### HTTP Router

The Gateway exposes two logical protocol surfaces:

**HTTP (Plain Text, Port 8080)**

Routes: health checks, bootstrap and PKI discovery, token-scoped CLI recovery, platform-enrollment request/status/complete, g8e binary download, deploy scripts, and catch-all redirect to HTTPS. No mTLS, MCP, or governed API routes.

**HTTPS (TLS + Application Auth, Port 8443)**

Client certificates are optional at the TLS handshake (allowing public browser and bootstrap assets) but application middleware requires verified mTLS identity for governance routes. The router classifies routes into four auth modes:

| Auth Mode | Routes | Identity |
| --- | --- | --- |
| **Public** | Health, state, PKI discovery, landing, logout, Console SPA, browser passkey registration, bootstrap/device enrollment, token-scoped CLI recovery | None required |
| **mTLS-only** | Data, blob, and KV stores, operator management, governance, consensus, audit, pub/sub, SSE push, PKI management, passkey CLI status, enrollment token generation, CLI rotation, CLI recovery approval via headless `approve-cli` endpoint | Client certificate verified |
| **Web-session-only** | User profile, approvals, passkey credential management, CLI recovery approval via browser Console SPA | Web session cookie with active user |
| **Dual** | SSE stream and event endpoints | Either valid client certificate or web-session cookie |

The full route list and auth assignments are part of the wire contract in [protocol/docs/spec.md](../../protocol/docs/spec.md).

### 5-Layer Verification Sequence

Every transaction to `POST /api/v1/governance/envelopes` passes through all five layers. The Gateway (PDP) owns or coordinates L1-L3. The Operator substrate (PEP) owns L4-L5 enforcement. Remote Governed Operators independently re-verify L1-L3 proofs before L4-L5.

**L1 Doctrine (Technical Bedrock) — Gateway**

Enforced by [internal/services/governance/l1_doctrine.go](internal/services/governance/l1_doctrine.go). Validates forbidden patterns (such as `sudo`, `rm -rf /`), blacklists, whitelists, and performs MITRE threat detection on incoming payloads. Configuration loaded from `--doctrine-dir` (env: `G8E_DOCTRINE_DIR`); defaults to hardcoded patterns only.

**L2 Consensus (Consensus Deliberation) — Gateway**

Implemented in [internal/services/governance/](internal/services/governance/). The Gateway delegates L2 deliberation to an enrolled Consensus service (not self-signing). The Consensus evaluates the transaction and produces `L2Vote` entries (Ed25519 signatures over transaction hash) from its member agents. Under `consensus` and `notary` postures, the Gateway calls the Consensus's `Deliberate` endpoint and attaches returned votes to the envelope. L4 Warden verifies quorum of valid signatures against `ConsensusPolicy` in the consensus store.

**L3 Notary (Human Authorization) — Gateway**

Enforced by [internal/services/governance/l3_notary.go](internal/services/governance/l3_notary.go). Layered model: WebAuthn passkeys provide human authorization, CLI mTLS session verification additionally binds CLI proofs when `mtls_cert_fingerprint` is present, and Operator proof path uses mTLS identity only (passkey unavailable for operators). JWT sessions use JIT user provisioning with JWT signature validation at the Gateway; the Operator receives pre-validated enriched metadata only.

**L4 Warden (Pre-Dispatch Gating) — Operator**

Runs on Operator substrate (in-process for gateway-host operations, remote for managed-host operations). Implemented in [internal/services/governance/l4_warden.go](internal/services/governance/l4_warden.go). Enforces final pre-execution gates: transaction hash match, expiry check, nonce/replay prevention (sliding-window via replay store), state root validation (if provided), and L2/L3 signature verification against trusted signer keys.

**L5 Actuator (Execution and Receipt) — Operator**

Runs on Operator substrate. Implemented in [internal/services/governance/l5_actuator.go](internal/services/governance/l5_actuator.go). Fails closed across evidence, persistence, commitment, and dispatch: (1) signs and persists complete protojson `ActionReceipt` with L4 stage evidence before side effect, (2) builds and appends signed `CommitmentAttestation` against SQLite chain head under write lock, records chain hashes in receipt evidence, (3) dispatches verified payload through scoped capability to downstream execution handler (MCP server or similar), (4) adds L5 outcome evidence and state transitions, signs and persists final receipt, attaches signed `ReceiptPersistenceAttestation` proving audit-record association.

### MCP Endpoint and Native Tools

The Gateway implements a unified MCP (Model Context Protocol) endpoint at `/api/v1/mcp/` accepting POST requests with JSON-RPC 2.0 messages and dispatching by method field. Methods include: `initialize`, `ping`, `tools/list`, `tools/call`, `resources/list`, `resources/templates/list`, `resources/read`, `prompts/list`, `prompts/get`, `a2a/call`. GET requests support SSE for streaming.

The native tool registry ([internal/services/mcp/native_tool_registry.go](internal/services/mcp/native_tool_registry.go)) maintains 31 tools across eight categories, all registered at startup via explicit registration. All tools enforce fail-closed input validation: SQL queries reject empty queries and trailing semicolons; URLs reject localhost/loopback/private IPs to prevent SSRF; paths prevent directory traversal; protocols validate against (tcp, udp, tcp6, udp6, raw) to prevent injection; hostnames prevent shell injection; operators validate binary paths and arguments.

Tool categories:

- **Database**: `db_discover_topology`, `db_index_triage`, `db_isolated_read`, `db_query_validate`
- **Filesystem**: `fs_disk_profile`, `fs_disk_usage`, `fs_file_checksum`, `read_file`, `log_stream_filter`
- **Network**: `net_endpoint_ping`, `net_http_probe`, `net_socket_audit`, `net_dns_resolve`, `net_ssh_known_hosts`, `tls_cert_inspect`
- **Process**: `proc_metric_top`, `proc_signal_safe`, `proc_tree`
- **System**: `sys_oom_detect`, `sys_info`, `sys_env_vars`, `sys_service_status`, `sys_container_status`, `sys_time_clock`
- **Configuration**: `config_diff_mask`
- **Cloud**: `cloud_metadata`
- **Kubernetes**: `k8s_inspect`
- **Git**: `git_ops`
- **Operator**: `operator_deploy`
- **Execution**: `run_shell_command`
- **Audit**: `audit_receipt_list`, `audit_receipt_get`

### Agent Integration

The Gateway provides zero-config ingress for agentic CLI coding tools (Claude Code, OpenAI Codex, Goose, Gemini CLI, Devin CLI) via MCP agent subcommands. Each agent has its native tools disabled at launch, forcing all I/O through the g8e MCP gateway for full L1-L5 governance enforcement.

Commands:

- `g8e mcp agent list` — Lists supported agent binaries: Claude Code, OpenAI Codex, Devin CLI, Goose, Gemini CLI.
- `g8e mcp agent show <agent>` — Prints MCP client configuration (g8e.local mTLS, IP address mTLS, stdio transport).
- `g8e mcp agent run <agent> [--verify] [-- <args...>]` — Launches an agent with automatic MCP configuration and native tool disabling. The stdio proxy bridges stdio MCP transport to gateway mTLS HTTPS endpoint. Runtime verification (`--verify`, enabled by default) checks that tool-disabling configuration was written correctly; use `--verify=false` to skip for pre-validated configs. External MCP servers connect via gateway downstream egress flags (`--mcp-downstream-cmd`, `--mcp-downstream-url`).

When the Gateway returns an L3 approval response, the stdio proxy auto-opens a browser, subscribes to SSE stream (`GET /api/v1/sse/stream`), waits for the `approval.completed` event (3-minute timeout; Gateway approval request TTL is 2 minutes), and re-sends the original request.

### Incremental State Tracking

The Gateway's SQLite state-root service uses incremental state tracking to avoid unnecessary full recomputation. A monotonically increasing `state_version` counter tracks changes across data stores. Triggers automatically increment this counter on document, KV store, and blob mutations. The Gateway queries the current version before expensive state-root calculations, skipping computation when version unchanged.

Two state tiers for Merkle root computation:

- **Bound State** (authoritative): documents, bound KV entries, bound blobs, token keymap. Bound state root gates transaction admission and is the freshness root in-flight envelopes depend on.
- **Observed State** (telemetry): observed KV entries and blobs. Hashed into separate observed state root chained into audit ledger but does not gate transaction admission. Prevents telemetry churn from invalidating in-flight envelopes.

### Health Endpoints

**Main Health Check (HTTPS, port 8443)**

Full readiness check: optional readiness callback, platform settings document availability, state root calculation success. Response includes `status`, `mode`, `version`, `pid` (OS process ID), `governance_ready` (whether governance pipeline initialized), `state_merkle_root` (current state root). Unauthenticated to bypass auth middleware.

**Bootstrap Health Check (HTTP, port 8080)**

Lighter check: readiness callback only, skipping platform settings and state root. Response includes `status`, `mode`, `version`, `pid`, `governance_ready`. Suitable for initialization monitoring before database fully configured.

## Anti-patterns

- Running `gw restart` when the launch profile in `.g8e/pids/operator-launch-profile.json` is missing, malformed, or unknown-version without first fixing the underlying cause (INV-GW-06).
- Changing auth mode assignments in the HTTP router without updating the wire contract documentation in [protocol/docs/spec.md](../../protocol/docs/spec.md) (INV-GW-07).
- Adding new native tools to the registry without implementing fail-closed input validation matching its category (SQL, URL, path, protocol, hostname, cloud metadata) (INV-GW-04).
- Running posture changes at runtime; posture must be set once at `gw start` and cannot be changed without restart (INV-GW-01).
- Dispatching transactions directly to L4/L5 without L1 Doctrine enforcement, even if L2/L3 are disabled by posture (INV-GW-03).
- Serving MCP, governance, or pub/sub routes on the plain-text HTTP port 8080 (INV-GW-02).

## Links out

- [g8e Protocol Specification](../../protocol/docs/spec.md) — Wire contract, governance hierarchy, route auth assignments.
- [Network Architecture](./network.md) — Transport, PKI hierarchy, identity management, SPIFFE trust domain, certificate enrollment.
- [Operator Architecture](./operator.md) — Sovereign host-side execution agent, MCP server, cross-gateway enrollment, cascading outbound-only topologies.
- [Storage Architecture](./storage.md) — SQLite persistence, audit vault, named volumes, runtime evidence ownership.
- [SSE Streaming](./sse.md) — Server-sent events, live audit streams, approval notifications.
- [Public Spectator Architecture](./public_spectator.md) — Anonymous mirror observation mode, private ingest listener, public read listener.
- [Evaluations](./evals.md) — Native execution-boundary suite, model campaign orchestration, agent state and run state producers.
