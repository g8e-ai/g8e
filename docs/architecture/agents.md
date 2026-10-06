---
doc_id: agents
title: AI Agents and the g8e Governance Boundary
audience: maintainers and coding agents
status: current
last_updated: 2026-10-06
version: v2.3.1
owners:
  - internal/cli/agent/
  - internal/cli/cmd/mcp/
  - internal/services/mcp/
  - ensemble/
related:
  - gateway.md
  - operator.md
  - governance.md
  - consensus.md
  - auth.md
  - network.md
  - sse.md
  - events.md
  - ensemble.md
when_to_read: Integrating external coding agents or agentic applications with g8e, understanding governance boundary separation, choosing MCP integration paths, and auditing agent execution.
do_not_use_for:
  - Gateway architecture and HTTP routing (see gateway.md)
  - Operator architecture and outbound transport (see operator.md)
  - Governance verification details (see governance.md)
  - Consensus policies and enrollment (see consensus.md)
---

# AI Agents and the g8e Governance Boundary

## Purpose

Describes three distinct meanings of "agent" in g8e: external coding agents (Claude Code, Codex, Devin CLI, Gemini CLI, Goose), the agent launcher workflow (`g8e mcp agent run`), and the optional first-party agentic ensemble (g8ee). Defines how agents interact with governance layers (L1–L5), which integration paths provide what level of protection, and why all three remain outside the trusted execution boundary. Agent reasoning, Tribunal consensus, and application memory do not produce protocol signatures or satisfy governance gates.

## Quick index

- [Purpose](#purpose)
- [Quick index](#quick-index)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

## Invariants

| ID | Rule |
| --- | --- |
| INV-AGT-01 | AI model outputs, ensemble Tribunal consensus, multi-model voting, and application memory are advisory only. They do not produce Ed25519 protocol signatures (L2) or WebAuthn proofs (L3) and cannot satisfy governance gates. All mutations still require the Gateway and executing Operator to enforce the active posture. |
| INV-AGT-02 | Every external agent (`claude`, `codex`, `devin`, `gemini`, `goose`) launched via `g8e mcp agent run <agent>` authenticates with an owner-approved platform application enrollment (`g8e auth enroll app <agent>`) whose mTLS certificate carries both the agent's SPIFFE app identity and the approving owner's SPIFFE user identity, cryptographically bound in URI SANs. Identity comes from the certificate, not headers. |
| INV-AGT-03 | Devin CLI does not expose flags or config options to disable native tools. Only operations routed through g8e MCP cross the governance boundary; Devin's native file, shell, and network access remain ungoverned side channels. |
| INV-AGT-04 | `g8e mcp stdio` fails closed immediately with `ErrIncompleteCredentialPair` when supplied a certificate without its key or vice versa. Partial credential pairs cannot degrade to a weaker tier. |
| INV-AGT-05 | Agents retain native tools (shell, file, network access) outside their configured MCP server unless the agent's launcher or configuration options actively disable them. Governance covers only MCP-routed operations; side channels remain agent-native. |
| INV-AGT-06 | Governed HTTP dispatch (`POST /api/v1/operators/commands`) cannot mint human L3 proofs and fails closed under `ratify` or `notary` postures when L3 proof is required. Direct envelopes and MCP/A2A submission paths support L3 suspension; HTTP dispatch does not. |
| INV-AGT-07 | Third-party MCP servers (subprocess or HTTP) are governed exclusively through Gateway downstream egress with full L1–L5 governance, envelope construction, and signed receipts. `g8e mcp agent run` is launcher-only and does not provide an external MCP wrapper or CLI reverse proxy. |
| INV-AGT-08 | The Go agent harness (`internal/tools/agent_harness/`) is a shared typed governed-client used by evaluation and integration workflows. It contains no model, Tribunal, or ReAct loop and is not g8ee. g8ee (`ensemble/`) MUST NOT import, invoke, or name the harness. Tribunal roles, including the Auditor (`protocol/models/agents/auditor.json`), belong to g8ee and MUST NOT use harness naming. |
| INV-AGT-09 | Every agent the launcher supports is exactly one `agent.Integration` entry in [internal/cli/agent/registry.go](../../internal/cli/agent/registry.go). Config writing, launch arguments, tool-lockdown level, `agent list`, `agent run`, `agent show`, and `agent verify` are all driven from that entry; no other code switches on an agent name. An unknown agent name fails closed with `ErrAgentNotFound` before anything is executed. Adding an agent means adding one registry entry plus its verify hooks. |

## Owned surfaces

| Surface | Path | Verify |
| --- | --- | --- |
| Agent integration registry | [internal/cli/agent/](../../internal/cli/agent/) (`registry.go`, `config.go`, `verify.go`) | Single source of truth for supported agents: config strategy, launch strategy, tool-lockdown level, verify hooks. `g8e mcp agent list` prints it |
| Agent verification without the agent | `g8e mcp agent verify <agent>`, [internal/cli/agent/verify.go](../../internal/cli/agent/verify.go) | Writes the config into an isolated temporary home, computes launch args, and runs the verify hooks. Never starts the agent binary or touches the real agent config, so it runs in CI |
| Agent launcher and stdio | [internal/cli/cmd/mcp/mcp.go](../../internal/cli/cmd/mcp/mcp.go) (`runMCPAgentRun`, `launchAgentWithGovernance`, `mcpStdioCmd`) | Enrolls human CLI, enrolls the agent application through owner-approved platform enrollment when no valid identity exists, configures MCP, verifies tool disabling, starts agent |
| MCP client config templates | [internal/services/mcp/config.go](../../internal/services/mcp/config.go), `g8e mcp agent show <agent>` | Generates MCP client configuration templates (mTLS, direct IP, Stdio) for connecting external coding agents and IDEs to the Gateway |
| MCP native tools | [internal/services/mcp/native_tool_registry.go](../../internal/services/mcp/native_tool_registry.go) | 32 tools across database, filesystem, system, network, cloud/ops, and audit categories |
| Tool interception config | [internal/cli/agent/config.go](../../internal/cli/agent/config.go) ([`Integration.WriteConfig`](../../internal/cli/agent/config.go)) | Per-agent disabling: Claude/Codex (flags), Goose (extensions), Gemini (settings), Devin (MCP server list) |
| g8ee ensemble | [ensemble/](../../ensemble/) (Python), [ensemble/app/main.py](../../ensemble/app/main.py) | Triage, Tribunal, ReAct tool loops, outbound dispatch, SSE events |
| Go agent harness (not an agent, not g8ee) | [internal/tools/agent_harness/](../../internal/tools/agent_harness/) | Typed Gateway client and configuration helpers used by evaluation and integration code. `git grep -i agent_harness ensemble/` returns nothing |

## Procedures

### Choose an integration path

Each path provides different governance guarantees:

- **`g8e mcp agent run <agent>`** — Named-agent launch with full L1–L5 governance, native-tool disabling, automatic human CLI enrollment, and an enrolled application identity. The Gateway constructs envelopes, coordinates L2/L3 when required, and produces signed receipts. Use for Claude Code, Codex, Devin CLI, Gemini CLI, or Goose when local full governance is required.

- **Gateway downstream egress (`--mcp-downstream-cmd` / `--mcp-downstream-url`)** — Connect third-party MCP servers (subprocess or HTTP) as downstream targets of the Gateway. Every `tools/call` traverses the complete L1–L5 governance pipeline and produces signed receipts in the audit vault.

- **Direct MCP client to `/mcp`** — An MCP client connects directly to the Gateway `/mcp` endpoint with enrolled mTLS credentials. The Gateway constructs `GovernanceEnvelope` for every `tools/call`, `resources/read`, or `prompts/get` request and may coordinate L2/L3 when the active posture requires it.

- **A2A call to `/api/v1/a2a/call`** — Authenticated REST call with a protobuf-serialized payload. The Gateway constructs an `A2A_CALL` envelope and manages L1–L3 according to the active posture.

- **Governed HTTP dispatch to `/api/v1/operators/commands`** — Registered request `event_type` and serialized payload from an enrolled app. The Gateway validates the session, derives `action_type`, and constructs the envelope without synthesizing missing L2 votes or suspending for L3 approval. Fails closed under `ratify` or `notary` when L3 proof is required.

- **Direct envelope to `/api/v1/governance/envelopes`** — Complete `GovernanceEnvelope` with all required L2/L3 proofs already attached. Reserved for authorized CLI and Operator transport identities; app certificates are rejected.

### Launch an agent for local governance

```bash
# Start Gateway with a posture (once):
g8e gw start --posture doctrine

# If the launcher has to start the Gateway it uses --posture, else the previous managed
# Gateway's posture, else doctrine. A running Gateway keeps its posture; an explicit
# --posture that does not match it fails closed:
g8e mcp agent run claude --posture notary

# Launch a supported agent with automatic enrollment and tool disabling:
g8e mcp agent run claude      # Claude Code
g8e mcp agent run codex       # OpenAI Codex
g8e mcp agent run devin       # Devin CLI
g8e mcp agent run gemini      # Gemini CLI
g8e mcp agent run goose       # Goose

# Pass additional arguments to the agent:
g8e mcp agent run claude -- -p "fix the failing tests"

# Skip tool interception verification (not recommended):
g8e mcp agent run claude --verify=false

# Check an agent's launcher config and lockdown without installing or starting it (CI-safe):
g8e mcp agent verify claude

# Display MCP client configuration (mTLS, IP, Stdio) for manual setup:
g8e mcp agent show claude

# Govern external MCP server via Gateway downstream egress:
g8e gw start --mcp-downstream-cmd npx --mcp-downstream-args '-y,@modelcontextprotocol/server-filesystem,/tmp'
g8e gw start --mcp-downstream-url http://localhost:3000/mcp
```

### Query agent audit trails

```bash
# List audit events for a specific operator session:
g8e gw data audit list --operator-session-id <operator-session-id>

# Summarize audit activity (optionally filtered by operator session):
g8e gw data audit summary --operator-session-id <operator-session-id>

# Query audit events filtered directly by agent application identity:
g8e audit events --app spiffe://g8e.local/app/claude

# List signed transaction receipts filtered directly by agent application identity:
g8e audit receipts --app spiffe://g8e.local/app/claude
```

The `--operator-session-id` flag scopes `gw data audit` queries to an Operator session. The acting agent application identity is recorded on each transaction and `ActionReceipt` as `acting_app_id` (the app SPIFFE ID, for example `spiffe://g8e.local/app/claude`). The top-level `g8e audit events` and `g8e audit receipts` commands accept `--app <spiffe-id>` to query directly by application identity without requiring an Operator session ID.

## Trust and Execution Boundaries

The Governance Gateway is the Policy Decision Point. It authenticates clients via mTLS (not headers), constructs `GovernanceEnvelope` containers for MCP and A2A calls, binds the current state and active posture, coordinates L2 consensus when required, and suspends transactions for L3 human approval when applicable.

The Governed Operator is the Policy Execution Point for its own runtime. A remote Operator opens an outbound mTLS connection to the Gateway, subscribes to its exact session-specific command channel, verifies each envelope locally at L4, and executes accepted operations through L5. It opens no inbound management port. The Gateway's in-process Operator substrate executes only against the Gateway host; a remote Operator executes only against its own host.

The Gateway also contains an in-process Operator substrate. Gateway-local MCP and A2A calls target `embedded-operator` before hashing; caller application and human identity remain in `acting_app_id` and `requestor_user_id`. L3 approval resume attaches proof without changing hashed fields, and the notary verifies the human `requestor_user_id`. Direct envelopes received by the Gateway execute through the in-process L4/L5 path only when their target belongs to the embedded service (INV-GOV-WARD-07); the unbound app document-write exception is defined by INV-AUTH-ID-05. An external agent launching via `g8e mcp agent run` uses the in-process path for MCP calls. Host commands dispatched by g8ee through `POST /api/v1/operators/commands` route through the outbound Operator path instead, so the remote Operator performs L4/L5 verification and execution.

This boundary governs only operations that traverse a g8e ingress (MCP, A2A, HTTP dispatch, direct envelope). It does not sandbox an AI process, restrict native tools that remain enabled in the client, or govern network access, shell access, or filesystem access through client-native channels.

## Agent Integration Paths

| Path | Input | Governance | Execution |
| --- | --- | --- | --- |
| **Launched agent** | `g8e mcp agent run <agent>` with automatic MCP configuration and an enrolled application identity | JSON-RPC MCP requests (tools/call, resources/read, prompts/get) are translated into `GovernanceEnvelope`, processed through L1–L5, coordinated with L2/L3 when posture requires. List and discovery methods do not execute tools. Signed `ActionReceipt` returned on success or L1/L2 rejection. | Gateway in-process Operator, built-in tool, or configured downstream MCP/A2A service |
| **Gateway downstream egress** | Configured downstream via `--mcp-downstream-cmd` or `--mcp-downstream-url` on Gateway | Complete L1–L5 governance, envelope construction, posture enforcement, and signed receipts. Tools and resources discovered from downstream. | Downstream subprocess or HTTP MCP server |
| **Gateway MCP endpoint** | JSON-RPC methods at `/mcp` with enrolled mTLS credentials | Gateway constructs `GovernanceEnvelope`, applies L1 Doctrine, attempts L2 deliberation when posture requires it, suspends for L3 approval when applicable, processes through L1–L5 | Gateway in-process Operator, built-in tool, or configured downstream MCP/A2A service |
| **A2A call** | JSON-RPC `a2a/call` at `/api/v1/a2a/call` with enrolled mTLS app credentials | Gateway constructs `A2A_CALL` envelope, applies L1–L3 per posture, suspends for L3 when needed, processes through L1–L5 | Configured downstream A2A service through Gateway Actuator path |
| **Governed HTTP dispatch** | Registered request `event_type` and serialized payload at `POST /api/v1/operators/commands` from enrolled app | Gateway derives `action_type`, applies L1 screening, validates session, constructs envelope with identity/nonce/expiry/hash/posture. Does not synthesize L2 votes or suspend for L3. Fails closed if L3 proof required. | Bound outbound Operator (enforced per target session) |
| **Direct envelope** | Complete canonical `GovernanceEnvelope` at `/api/v1/governance/envelopes` with all proofs attached | Gateway verifies the supplied envelope but does not synthesize missing L2 or L3 proofs. Requires authorized CLI or Operator transport identity; rejects app certificates. | Gateway in-process Operator |

MCP and A2A are the normal client-facing surfaces when the Gateway must construct the envelope, coordinate L2 deliberation, or manage L3 approval. Under `consensus` or `notary` postures, MCP and A2A attempt L2 deliberation when a consensus policy is configured; if no valid votes are attached, L4 rejects the transaction. Direct envelope submission is reserved for clients that already hold authorized transport identity and can supply every required proof. Governed HTTP dispatch is suitable only when the active posture does not require L3 proof for the action type.

See [Build Apps](../guides/build_apps.md) for full guidance on choosing integration paths and managing application state.

## Agent Launcher Configuration

### Supported Agents

The launcher supports five external coding agents, each defined by one registry entry (INV-AGT-09) with agent-specific tool disabling. The registry records a lockdown level per agent: **strict** (every built-in tool disabled by the launcher) or **partial** (g8e is the only configured MCP server but native tools cannot be disabled).

- **Claude Code** (strict) — Receives a throwaway MCP configuration via `--mcp-config`; `--strict-mcp-config` ignores every other MCP server and `--disallowed-tools Bash,Read,Write,Edit,Glob,Grep,WebSearch,WebFetch` disables native tools. All I/O must traverse the g8e MCP server.
- **Codex (OpenAI)** (strict) — Same flags and throwaway configuration as Claude Code.
- **Goose** (strict) — Merges g8e as an extension into `~/.config/goose/config.yaml` (preserving other settings, backing up the previous file) and launches with `session --no-profile --with-extension` to disable all profile extensions. All I/O must traverse g8e MCP.
- **Gemini CLI** (strict) — Merges the g8e MCP server into `~/.gemini/settings.json` and sets `tools.core: []` (empty allowlist) to disable built-in tools. All I/O must traverse g8e MCP.
- **Devin CLI** (partial) — Configures `~/.config/devin/config.json` with g8e as the only MCP server. **Devin does not expose native-tool disabling flags**, so the launcher prints a warning on every launch. Only MCP-routed operations cross the governance boundary; Devin's native file, shell, and network access remain ungoverned side channels.

Before the agent starts, the registry's verify hooks confirm the written config and launch arguments actually carry the lockdown. A failure wraps `ErrToolInterceptionVerification` and the agent is not started (`--verify=false` skips this, which is not recommended).

### MCP Stdio Bridge and Credential Resolution

`g8e mcp stdio` is a credential-consuming stdio-to-HTTPS bridge. It answers MCP initialization locally, drops notifications that require no response, and proxies tool calls and other requests to the Gateway over TLS 1.3. It does not enroll users, open passkey ceremonies, or start the Gateway when invoked directly.

Credentials resolve in this order (first complete pair wins):

1. Application identity flag (`--app <name>`), resolving the managed certificates on disk (`.g8e/apps/<name>.crt`, `.g8e/apps/<name>.key`)
2. CLI client certificate and key flags (`--client-cert`, `--client-key`)
3. Enrolled CLI credentials on disk (`.g8e/cli.crt`, `.g8e/cli.key`)

Each tier must provide a complete certificate and key pair. An incomplete pair fails closed immediately with `ErrIncompleteCredentialPair` rather than attempting to degrade. Application credentials are owner-approved platform application enrollments (`g8e auth enroll app <name>`). The CA bundle resolves from its flag, then the enrolled trust bundle. The Gateway URL resolves from its flag, then the default HTTPS MCP URL (`https://g8e.local:8443/mcp`). No credential, path, or endpoint is read from the environment (INV-ENV-04): `g8e mcp agent run` writes `--app <agent>` into the agent's generated MCP configuration and passes nothing else to the agent process.

When L3 approval is required, the stdio bridge opens the approval page in the browser, waits for the matching `approval.completed` event over the authenticated SSE stream, and retries the original request. This automatic flow requires enrolled CLI credentials and a CLI session even when the MCP request itself uses an enrolled application identity.

### What `agent run` is not

`g8e mcp agent run` launches an external coding agent so its MCP traffic is governed. It is easy to conflate with three other things that also carry the word "agent":

| Not this | Why | Where it lives |
| --- | --- | --- |
| **g8ee** | g8ee is a first-party application that integrates over governed HTTP dispatch (`POST /api/v1/operators/commands`), never as an MCP stdio child of the launcher. | [First-Party Agentic Ensemble](#first-party-agentic-ensemble-g8ee), [Ensemble Architecture](./ensemble.md) |
| **Agent harness personas** | Personas (`claude-desktop`, `cursor`, ...) impersonate MCP clients against a real Gateway to test Gateway behavior ([internal/tools/agent_harness/client/](../../internal/tools/agent_harness/client/)). They do not exercise launcher output. Launcher config and tool lockdown are verified by isolated unit tests ([internal/cli/agent/verify_test.go](../../internal/cli/agent/verify_test.go), [internal/cli/cmd/mcp/mcp_agent_proxy_test.go](../../internal/cli/cmd/mcp/mcp_agent_proxy_test.go)) and `g8e mcp agent verify <agent>`, while [test/e2e/ensemble_chat_e2e_test.go](../../test/e2e/ensemble_chat_e2e_test.go) exercises the g8ee chat path. Neither duplicates the other. | [internal/tools/agent_harness/](../../internal/tools/agent_harness/), INV-AGT-08 |
| **Eval campaigns** | Campaigns score candidate models by driving the g8ee chat turn and importing its trace. They never validate external agent launcher configs. | [Evals](./evals.md) |

## Five-Layer Governance Enforcement

Every governed MCP tool call follows the same L1–L5 verification pipeline:

### L1 Doctrine

Decodes the typed payload and applies protobuf field constraints, forbidden-pattern rules, and MITRE ATT&CK-oriented threat detection. L1 is mandatory in every posture. The executing Operator performs L1 validation locally before dispatch.

### L2 Consensus

Verifies Ed25519 signatures from enrolled consensus members over the transaction hash and their decision. Required under `consensus` and `notary` postures; audited only under `doctrine` and `ratify`. The Gateway attempts L2 deliberation under `consensus` and `notary`; direct envelopes and governed HTTP dispatch do not receive votes automatically. See [Consensus](./consensus.md) for enrollment, deliberation, and vote verification.

### L3 Notary

Authorizes mutations under `ratify` and `notary` postures. Gateway MCP and A2A flows suspend for WebAuthn approval when the proof is missing; direct envelopes and governed HTTP dispatch must arrive with the required proof already attached. Read-only actions do not require L3 approval under any posture.

### L4 Warden

Reserves the nonce for durable replay prevention, checks expiry, decodes and validates the typed payload, recomputes the transaction hash, verifies the current state root, and evaluates posture-required L2 and L3 evidence. Universal check failures and required proof failures reject the transaction and produce signed rejection evidence when the Actuator is available.

### L5 Actuator

Signs and persists an `EXECUTING` receipt before invoking the handler. Rehydrates scrubbed payload data at the execution site using local vault keys, mints a transaction-bound just-in-time capability, invokes the handler, dissolves the capability, and signs the final `COMPLETED` or `FAILED` receipt. Appends a signed commitment when the SQL commitment ledger is available.

| Posture | L1 | L2 | L3 (mutations) |
| --- | --- | --- | --- |
| `doctrine` | Enforced | Audited | Audited |
| `consensus` | Enforced | Enforced (quorum) | Audited |
| `ratify` | Enforced | Audited | Enforced |
| `notary` | Enforced | Enforced (quorum) | Enforced |

## First-Party Agentic Ensemble (g8ee)

g8ee is an optional first-party Python 3.12 / FastAPI client application that implements conversational workflows outside the trusted execution boundary:

- **Triage and model selection** — Classifies each conversation turn and routes queries to either the Dash (fast-path) or Sage (primary reasoning) assistant.
- **ReAct tool loops** — Executes sequential tool-calling loops. Each tool result returns to the model for the next turn; reaching the configured tool-turn limit requires an explicit user continuation decision.
- **Tribunal command vetting** — Host-operation requests pass through a five-member Tribunal (Axiom, Concord, Variance, Pragma, Nemesis) for independent analysis. Tribunal consensus is advisory only and does not produce protocol L2 signatures.
- **Marshal risk analysis** — Aggregates Tribunal results, detects clustering, and performs threat assessment before dispatch to the Operator.
- **Application approvals** — g8ee's own application-level approval service for auto-approved commands. This is application-owned policy, not protocol L3 authorization.
- **Outbound dispatch** — For host operations, g8ee dispatches via `GatewayOperatorClient.dispatch()` to `POST /api/v1/operators/commands` with a registered request `event_type`. For designated application records (cases, investigations, memories), it constructs direct envelopes and submits to `POST /api/v1/governance/envelopes`. For audit trails, it posts to `POST /api/v1/audit/records`.
- **SSE event publishing** — Publishes typed progress, approval, command, and result events through the Gateway SSE push API for browser and CLI consumption. SSE is delivery telemetry, not governance state.

Tribunal consensus, model outputs, model confidence scores, conversation history, and application approvals remain application-owned and do not produce protocol signatures. See [Ensemble Architecture](./ensemble.md) for deployment, connection model, and event schema. See [g8ee Personas & Agents](../ensemble/agents.md) for Tribunal member roles, Auditor, and Marshal specifications.

## Results, Receipts, and Audit Evidence

A completed Gateway MCP tool call returns tool content plus a cryptographic signed `ActionReceipt`. Calls rejected after L1 or L2 validation return the signed stage evidence in the JSON-RPC error `data` field. The executing Actuator stores the complete receipt in its local SQL audit vault. A remote Operator additionally publishes signed receipts to the Gateway receipt channel; the Gateway verifies the signer and mirrors receipts for centralized access. That mirror is best-effort and does not replace the Operator's local authoritative record.

Governed file mutations record file-mutation evidence and ledger hashes when the file ledger is active. The SQL commitment chain covers all admitted executions independently of the file ledger.

For audit queries, use `g8e gw data audit list` and `g8e gw data audit summary` scoped by `--operator-session-id`, or `g8e audit events` and `g8e audit receipts` scoped by `--app <spiffe-id>`. See [Gateway Architecture](./gateway.md) for audit API details.

## Anti-patterns

- Treating AI model outputs, ensemble Tribunal consensus, or application approvals as protocol L2 Consensus signatures or L3 Notary proofs (INV-AGT-01). They are advisory and do not authorize operations.
- Passing partial credential pairs (e.g., `--client-cert` without `--client-key`) to `g8e mcp stdio`, expecting it to degrade gracefully. It fails closed immediately (INV-AGT-04).
- Expecting `g8e mcp agent run` to wrap arbitrary third-party MCP commands or URLs. Third-party MCP servers must be configured as downstream egress on the Gateway (`--mcp-downstream-cmd` or `--mcp-downstream-url`) to receive full L1–L5 governance and signed audit receipts (INV-AGT-07).
- Dispatching mutations via `POST /api/v1/operators/commands` under `ratify` or `notary` postures without pre-obtained L3 proofs. The HTTP dispatch path cannot suspend for user approval; it fails closed when L3 is required (INV-AGT-06).
- Relying on Devin CLI's governance boundary when Devin retains native shell, file, and network tools outside MCP. Only MCP-routed operations cross the boundary (INV-AGT-03, INV-AGT-05).
- Treating Server-Sent Events (SSE) as durable governance state or execution evidence. SSE is transient delivery telemetry and does not modify the state Merkle root.
- Hard-coding agent identity or assuming agent names in `g8e mcp agent run` don't change. Use `g8e mcp agent list` to discover supported agent binaries.

## Links out

- [Gateway Architecture](./gateway.md) — Policy Decision Point, operational modes, MCP endpoint, HTTP routing, and session management.
- [Operator Architecture](./operator.md) — Policy Execution Point, outbound mTLS transport, native tools, and L4/L5 execution.
- [Governance Architecture](./governance.md) — Five-layer verification pipeline, posture enforcement rules, and per-posture behavior.
- [Consensus Architecture](./consensus.md) — Protocol L2 consensus, Ed25519 signer policies, deliberation mechanics, and vote verification.
- [Authentication and Authorization](./auth.md) — mTLS identities, platform-enrolled application credentials, CLI sessions, passkey enrollment, and WebAuthn ceremonies.
- [Network Architecture](./network.md) — PKI hierarchy, TLS 1.3/mTLS configurations, pub/sub transport, and Operator channels.
- [Storage Architecture](./storage.md) — SQLite database topologies, audit vaults, evidence persistence, and commitment ledgers.
- [SSE Streaming](./sse.md) — Server-Sent Events architecture for approval notifications and application telemetry.
- [Event and Action Protocol](./events.md) — Authoritative event registry, action-type derivation, and transport ownership.
- [Ensemble Architecture](./ensemble.md) — First-party g8ee deployment, connection model, and event schema.
- [g8ee Personas & Agents](../ensemble/agents.md) — g8ee persona hierarchy, Tribunal member roles, Auditor, and Marshal specifications.
- [Build Apps Guide](../guides/build_apps.md) — Guidance on choosing integration paths, managing application state, and designing for governed platforms.
